package verifier

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/validation"
)

const (
	cookieProviderInterface = `Shopware\Storefront\Framework\Cookie\CookieProviderInterface`
	cookieGroupCollectEvent = `Shopware\Core\Content\Cookie\Event\CookieGroupCollectEvent`
	cookieListenerXMLTag    = `<tag name="kernel.event_listener" event="` + cookieGroupCollectEvent + `"/>`
	cookieListenerYAMLTag   = `{ name: kernel.event_listener, event: ` + cookieGroupCollectEvent + ` }`
)

var (
	phpNamespacePattern    = regexp.MustCompile(`(?m)^\s*namespace\s+([\w\\]+)\s*;`)
	phpClassPattern        = regexp.MustCompile(`(?m)^\s*(?:(?:final|readonly|abstract)\s+)*class\s+(\w+)`)
	cookieListenerPattern  = regexp.MustCompile(`function\s+__invoke\s*\(\s*\\?(?:[\w\\]*\\)?CookieGroupCollectEvent\s+\$`)
	xmlServiceTagPattern   = regexp.MustCompile(`(?s)<(/?)service\b([^>]*?)(/?)>`)
	xmlAttributePattern    = regexp.MustCompile(`\s+([\w:-]+)\s*=\s*"([^"]*)"`)
	xmlDecorationAttribute = regexp.MustCompile(`\s+(?:decorates|decoration-inner-name|decoration-priority|decoration-on-invalid)\s*=\s*"[^"]*"`)
)

// CookieListenerWiring updates the service definitions of cookie providers
// that Rector turned from CookieProviderInterface decorators into
// CookieGroupCollectEvent listeners. Rector only changes PHP files, so without
// this step the container would still decorate the interface with a class that
// no longer implements it.
//
// The step edits services.xml and services.yaml, which the symfony-xml fixer
// may convert at the same time. It is therefore not registered as a parallel
// fixer and runs after all of them, see FollowUpFixTools.
type CookieListenerWiring struct{}

func (CookieListenerWiring) Name() string {
	return "cookie-listener-wiring"
}

// FollowUpFixTools returns the fixers that must run after the selected fixers
// have finished.
func FollowUpFixTools(selected ToolList[FixTool]) ToolList[FixTool] {
	if slices.ContainsFunc(selected, func(tool FixTool) bool { return tool.Name() == Rector{}.Name() }) {
		return ToolList[FixTool]{CookieListenerWiring{}}
	}

	return nil
}

func (CookieListenerWiring) Fix(_ context.Context, config ToolConfig, execution *ToolExecution) ToolSummary {
	listeners, err := findCookieListeners(config.SourceDirectories)
	if err != nil {
		execution.Fail(&ToolError{Err: fmt.Errorf("cannot search cookie listeners: %w", err)})
		return ToolSummary{}
	}
	if len(listeners) == 0 {
		return ToolSummary{SkipReason: "no cookie provider was migrated"}
	}

	files, err := serviceConfigFiles(pluginConfigDirs(config))
	if err != nil {
		execution.Fail(&ToolError{Err: fmt.Errorf("cannot search service configuration: %w", err)})
		return ToolSummary{}
	}

	wired := map[string]bool{}
	changed := 0
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			execution.Fail(&ToolError{Err: fmt.Errorf("cannot read service configuration: %w", err)})
			continue
		}
		if !strings.Contains(string(content), "CookieProviderInterface") {
			continue
		}

		var updated string
		var skipped []string
		if filepath.Ext(file) == ".xml" {
			updated = rewireCookieListenersXML(string(content), listeners, wired)
		} else {
			updated, skipped, err = rewireCookieListenersYAML(string(content), listeners, wired)
			if err != nil {
				execution.Fail(&ToolError{Err: fmt.Errorf("cannot update %s: %w", file, err)})
				continue
			}
		}

		for _, class := range skipped {
			execution.AddFindings(validation.CheckResult{
				Path: validation.NormalizeSourcePath(file, config.RootDir), Severity: validation.SeverityWarning,
				Identifier: "cookie-listener-wiring/unsupported",
				Message:    fmt.Sprintf("Cannot update the service definition of %s, its YAML uses flow style or an unexpected tags shape; remove the decoration and tag it as kernel.event_listener for %s manually", class, cookieGroupCollectEvent),
			})
		}

		if updated == string(content) {
			continue
		}
		if err := os.WriteFile(file, []byte(updated), 0o644); err != nil {
			execution.Fail(&ToolError{Err: fmt.Errorf("cannot write service configuration: %w", err)})
			continue
		}
		changed++
	}

	for class, file := range listeners {
		if wired[class] {
			continue
		}
		execution.AddFindings(validation.CheckResult{
			Path: validation.NormalizeSourcePath(file, config.RootDir), Severity: validation.SeverityWarning,
			Identifier: "cookie-listener-wiring/not-found",
			Message:    fmt.Sprintf("No service decorating %s found for %s; make sure it is registered as kernel.event_listener for %s", cookieProviderInterface, class, cookieGroupCollectEvent),
		})
	}

	return ToolSummary{Note: fileChangeNote(changed, "updated")}
}

// findCookieListeners returns the classes, mapped to their file, that listen to
// CookieGroupCollectEvent with __invoke and no longer mention
// CookieProviderInterface, which is the output of the Rector migration.
func findCookieListeners(sourceDirectories []string) (map[string]string, error) {
	listeners := map[string]string{}

	for _, dir := range sourceDirectories {
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == "vendor" || entry.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ".php" {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			source := string(content)
			if !cookieListenerPattern.MatchString(source) || strings.Contains(source, "CookieProviderInterface") {
				return nil
			}

			class := phpClassPattern.FindStringSubmatch(source)
			if class == nil {
				return nil
			}
			name := class[1]
			if namespace := phpNamespacePattern.FindStringSubmatch(source); namespace != nil {
				name = namespace[1] + `\` + name
			}
			listeners[name] = path

			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}

	return listeners, nil
}

// serviceConfigFiles returns all XML and YAML files in the config directories,
// including imported files in subdirectories.
func serviceConfigFiles(configDirs []string) ([]string, error) {
	var files []string

	for _, dir := range configDirs {
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			switch filepath.Ext(path) {
			case ".xml", ".yaml", ".yml":
				if !entry.IsDir() {
					files = append(files, path)
				}
			}
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}

	return files, nil
}

func normalizeServiceID(id string) string {
	return strings.TrimPrefix(id, `\`)
}

// rewireCookieListenersXML edits the XML as text, so formatting and comments
// of the remaining file are kept.
func rewireCookieListenersXML(content string, listeners map[string]string, wired map[string]bool) string {
	var out strings.Builder
	offset := 0

	tags := xmlServiceTagPattern.FindAllStringSubmatchIndex(content, -1)
	for i := 0; i < len(tags); i++ {
		tag := tags[i]
		if content[tag[2]:tag[3]] == "/" {
			continue
		}

		attributes := map[string]string{}
		for _, attribute := range xmlAttributePattern.FindAllStringSubmatch(content[tag[4]:tag[5]], -1) {
			attributes[attribute[1]] = attribute[2]
		}

		class := attributes["class"]
		if class == "" {
			class = attributes["id"]
		}
		class = normalizeServiceID(class)
		if normalizeServiceID(attributes["decorates"]) != cookieProviderInterface || listeners[class] == "" {
			continue
		}

		innerName := attributes["decoration-inner-name"]
		if innerName == "" {
			innerName = attributes["id"] + ".inner"
		}

		indent := lineIndent(content, tag[0])
		openTag := "<service" + xmlDecorationAttribute.ReplaceAllString(content[tag[4]:tag[5]], "")

		out.WriteString(content[offset:tag[0]])

		if content[tag[6]:tag[7]] == "/" {
			childIndent := indent + "    "
			out.WriteString(strings.TrimRight(openTag, " \t\n") + ">\n" + childIndent + cookieListenerXMLTag + "\n" + indent + "</service>")
			offset = tag[1]
			wired[class] = true
			continue
		}

		closing := findXMLServiceEnd(tags, i)
		if closing < 0 {
			out.WriteString(content[tag[0]:tag[1]])
			offset = tag[1]
			continue
		}

		body := content[tag[1]:tags[closing][0]]
		body = removeXMLInnerArgument(body, innerName)

		childIndent := indent + "    "
		if found := regexp.MustCompile(`\n([ \t]+)<`).FindStringSubmatch(body); found != nil {
			childIndent = found[1]
		}
		if !strings.Contains(body, `event="`+cookieGroupCollectEvent+`"`) {
			body = strings.TrimRight(body, " \t\n") + "\n" + childIndent + cookieListenerXMLTag + "\n" + indent
		}

		out.WriteString(openTag + ">" + body)
		offset = tags[closing][0]
		i = closing - 1
		wired[class] = true
	}

	out.WriteString(content[offset:])

	return out.String()
}

// findXMLServiceEnd returns the index of the closing tag that belongs to the
// service tag at index start, skipping inline services in arguments.
func findXMLServiceEnd(tags [][]int, start int) int {
	depth := 0
	for i := start + 1; i < len(tags); i++ {
		closing := tags[i][3] > tags[i][2]
		selfClosing := tags[i][7] > tags[i][6]
		switch {
		case closing && depth == 0:
			return i
		case closing:
			depth--
		case !selfClosing:
			depth++
		}
	}

	return -1
}

func removeXMLInnerArgument(body, innerName string) string {
	ids := []string{regexp.QuoteMeta(innerName), regexp.QuoteMeta(".inner")}
	pattern := regexp.MustCompile(`\n[ \t]*<argument\b[^>]*\bid="(?:` + strings.Join(ids, "|") + `)"[^>]*?(?:/>|>\s*</argument>)[ \t]*`)

	return pattern.ReplaceAllString(body, "")
}

func lineIndent(content string, position int) string {
	start := strings.LastIndex(content[:position], "\n") + 1
	line := content[start:position]

	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// rewireCookieListenersYAML uses the YAML node tree only to find line numbers
// and edits the file line by line, so formatting and comments are kept.
func rewireCookieListenersYAML(content string, listeners map[string]string, wired map[string]bool) (string, []string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		return "", nil, err
	}
	if len(document.Content) == 0 {
		return content, nil, nil
	}

	services := mappingValue(document.Content[0], "services")
	if services == nil || services.Kind != yaml.MappingNode {
		return content, nil, nil
	}

	lines := strings.Split(content, "\n")
	edits := yamlLineEdits{removed: map[int]bool{}, inserted: map[int][]string{}}
	var skipped []string

	for i := 0; i+1 < len(services.Content); i += 2 {
		key, definition := services.Content[i], services.Content[i+1]
		if definition.Kind != yaml.MappingNode {
			continue
		}

		class := key.Value
		if value := mappingValue(definition, "class"); value != nil {
			class = value.Value
		}
		class = normalizeServiceID(class)

		decorates := mappingValue(definition, "decorates")
		if decorates == nil || normalizeServiceID(decorates.Value) != cookieProviderInterface || listeners[class] == "" {
			continue
		}

		if !edits.rewire(key, definition) {
			skipped = append(skipped, class)
			continue
		}

		wired[class] = true
	}

	var out []string
	for index, line := range lines {
		if !edits.removed[index] {
			out = append(out, line)
		}
		out = append(out, edits.inserted[index]...)
	}

	return strings.Join(out, "\n"), skipped, nil
}

// yamlLineEdits collects line removals and insertions by zero-based line index.
type yamlLineEdits struct {
	removed  map[int]bool
	inserted map[int][]string
}

// rewire removes the decoration of one service definition and adds the event
// listener tag. It returns false for shapes that cannot be edited line by line.
func (e yamlLineEdits) rewire(key, definition *yaml.Node) bool {
	arguments := mappingValue(definition, "arguments")
	tags := mappingValue(definition, "tags")
	if definition.Style&yaml.FlowStyle != 0 ||
		(arguments != nil && arguments.Style&yaml.FlowStyle != 0) ||
		(tags != nil && (tags.Kind != yaml.SequenceNode || tags.Style&yaml.FlowStyle != 0 || len(tags.Content) == 0)) {
		return false
	}

	innerName := key.Value + ".inner"
	if value := mappingValue(definition, "decoration_inner_name"); value != nil {
		innerName = value.Value
	}

	for j := 0; j+1 < len(definition.Content); j += 2 {
		switch definition.Content[j].Value {
		case "decorates", "decoration_inner_name", "decoration_priority", "decoration_on_invalid":
			e.removed[definition.Content[j].Line-1] = true
		}
	}

	if arguments != nil {
		remaining := 0
		items := arguments.Content
		step := 1
		if arguments.Kind == yaml.MappingNode {
			step = 2
		}
		for j := 0; j+step-1 < len(items); j += step {
			value := items[j+step-1]
			if value.Kind == yaml.ScalarNode && (value.Value == "@"+innerName || value.Value == "@.inner") {
				e.removed[value.Line-1] = true
				continue
			}
			remaining++
		}
		if remaining == 0 {
			e.removed[mappingKey(definition, "arguments").Line-1] = true
		}
	}

	if tags != nil {
		itemIndent := strings.Repeat(" ", tags.Content[0].Column-3)
		last := lastLine(tags) - 1
		e.inserted[last] = append(e.inserted[last], itemIndent+"- "+cookieListenerYAMLTag)
	} else {
		childIndent := strings.Repeat(" ", definition.Content[0].Column-1)
		unit := strings.Repeat(" ", max(definition.Content[0].Column-key.Column, 2))
		last := lastLine(definition) - 1
		e.inserted[last] = append(e.inserted[last], childIndent+"tags:", childIndent+unit+"- "+cookieListenerYAMLTag)
	}

	return true
}

func mappingKey(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i]
		}
	}

	return nil
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}

	return nil
}

func lastLine(node *yaml.Node) int {
	line := node.Line
	for _, child := range node.Content {
		line = max(line, lastLine(child))
	}

	return line
}

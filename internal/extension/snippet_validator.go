package extension

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/wI2L/jsondiff"

	"github.com/shopware/shopware-cli/internal/validation"
)

const jsonFileExtension = ".json"

// snippetFileFilter determines if a JSON file should be included in snippet validation.
type snippetFileFilter func(path string, containingFolder string) bool

// storefrontSnippetFilter includes all JSON files in the snippet folder.
func storefrontSnippetFilter(_ string, _ string) bool {
	return true
}

// adminSnippetFilter only includes JSON files in folders named "snippet".
func adminSnippetFilter(_ string, containingFolder string) bool {
	return filepath.Base(containingFolder) == "snippet"
}

func validateStorefrontSnippets(ext Extension, check validation.Check) {
	validateSnippetsForExtension(ext, check, "snippet", storefrontSnippetFilter)
}

func validateAdministrationSnippets(ext Extension, check validation.Check) {
	validateSnippetsForExtension(ext, check, filepath.Join("app", "administration"), adminSnippetFilter)
}

// validateSnippetsForExtension validates snippets for an extension using the provided subpath and filter.
func validateSnippetsForExtension(ext Extension, check validation.Check, subPath string, filter snippetFileFilter) {
	rootDir := ext.GetRootDir()

	for _, val := range ext.GetResourcesDirs() {
		folder := filepath.Join(val, subPath)

		if err := validateSnippetsByPath(folder, rootDir, check, filter); err != nil {
			return
		}
	}

	for _, extraBundle := range ext.GetExtensionConfig().Build.ExtraBundles {
		bundlePath := extraBundle.ResolvePath(rootDir)
		folder := filepath.Join(bundlePath, "Resources", subPath)

		if err := validateSnippetsByPath(folder, rootDir, check, filter); err != nil {
			return
		}
	}
}

// validateSnippetsByPath validates snippet files in a folder using the provided filter.
func validateSnippetsByPath(folder, rootDir string, check validation.Check, filter snippetFileFilter) error {
	if _, err := os.Stat(folder); err != nil {
		return nil //nolint:nilerr
	}

	snippetFiles := make(map[string][]string)

	err := filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		if filepath.Ext(path) != jsonFileExtension {
			return nil
		}

		containingFolder := filepath.Dir(path)

		if !filter(path, containingFolder) {
			return nil
		}

		if _, ok := snippetFiles[containingFolder]; !ok {
			snippetFiles[containingFolder] = []string{}
		}

		snippetFiles[containingFolder] = append(snippetFiles[containingFolder], path)

		return nil
	})
	if err != nil {
		return err
	}

	for snippetFolder, files := range snippetFiles {
		if len(files) == 1 {
			// We have no other file to compare against
			continue
		}

		mainFile := findMainSnippetFile(files)

		if len(mainFile) == 0 {
			normalizedFolder := strings.ReplaceAll(snippetFolder, rootDir+"/", "")
			normalizedFile := strings.ReplaceAll(files[0], rootDir+"/", "")
			check.AddResult(validation.CheckResult{
				Path:       snippetFolder,
				Identifier: "snippet.validator",
				Message:    fmt.Sprintf("No en.json or en-GB.json file found in %s, using %s", normalizedFolder, normalizedFile),
				Severity:   validation.SeverityWarning,
			})
			mainFile = files[0]
		}

		mainFileContent, err := os.ReadFile(mainFile)
		if err != nil {
			return err
		}

		if !json.Valid(mainFileContent) {
			check.AddResult(validation.CheckResult{
				Path:       mainFile,
				Identifier: "snippet.validator",
				Message:    fmt.Sprintf("File '%s' contains invalid JSON", mainFile),
				Severity:   validation.SeverityError,
			})

			continue
		}

		for _, file := range files {
			// makes no sense to compare to ourself
			if file == mainFile {
				continue
			}

			compareSnippets(mainFileContent, mainFile, file, check, rootDir)
		}
	}

	return nil
}

func compareSnippets(mainFile []byte, mainFilePath, file string, check validation.Check, extensionRoot string) {
	checkFile, err := os.ReadFile(file)
	if err != nil {
		check.AddResult(validation.CheckResult{
			Path:       file,
			Identifier: "snippet.validator",
			Message:    fmt.Sprintf("Cannot read file '%s', due '%s'", file, err),
			Severity:   validation.SeverityError,
		})

		return
	}

	if !json.Valid(checkFile) {
		check.AddResult(validation.CheckResult{
			Path:       file,
			Identifier: "snippet.validator",
			Message:    fmt.Sprintf("File '%s' contains invalid JSON", file),
			Severity:   validation.SeverityError,
		})

		return
	}

	compare, err := jsondiff.CompareJSON(mainFile, checkFile)
	if err != nil {
		check.AddResult(validation.CheckResult{
			Path:       file,
			Identifier: "snippet.validator",
			Message:    fmt.Sprintf("Cannot compare file '%s', due '%s'", file, err),
			Severity:   validation.SeverityError,
		})

		return
	}

	normalizedMainFilePath := strings.ReplaceAll(mainFilePath, extensionRoot+"/", "")

	for _, diff := range compare {
		normalizedPath := strings.ReplaceAll(file, extensionRoot+"/", "")

		if diff.Type == jsondiff.OperationReplace && reflect.TypeOf(diff.OldValue) != reflect.TypeOf(diff.Value) {
			check.AddResult(validation.CheckResult{
				Path:       normalizedPath,
				Identifier: "snippet.validator",
				Message:    fmt.Sprintf("Snippet file: %s, key: %s, has the type %s, but in the main language it is %s", normalizedPath, diff.Path, reflect.TypeOf(diff.OldValue), reflect.TypeOf(diff.Value)),
				Severity:   validation.SeverityWarning,
			})
			continue
		}

		if diff.Type == jsondiff.OperationAdd {
			check.AddResult(validation.CheckResult{
				Path:       normalizedPath,
				Identifier: "snippet.validator",
				Message:    fmt.Sprintf("Snippet file: %s, missing key \"%s\" in this snippet file, but defined in the main language (%s)", normalizedPath, diff.Path, normalizedMainFilePath),
				Severity:   validation.SeverityWarning,
			})
			continue
		}

		if diff.Type == jsondiff.OperationRemove {
			check.AddResult(validation.CheckResult{
				Path:       normalizedPath,
				Identifier: "snippet.validator",
				Message:    fmt.Sprintf("Snippet file: %s, key %s is missing, but defined in the main language file", normalizedPath, diff.Path),
				Severity:   validation.SeverityWarning,
			})
			continue
		}
	}
}

// Search for the country-agnostic language file en.json
// If it isn't found search for the en-GB.json
func findMainSnippetFile(files []string) string {
	for _, file := range files {
		if strings.HasSuffix(filepath.Base(file), "en.json") {
			return file
		}
	}
	for _, file := range files {
		if strings.HasSuffix(filepath.Base(file), "en-GB.json") {
			return file
		}
	}
	return ""
}

package scaffolding

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"
)

const private = "custom/static-plugins"
const store = "custom/plugins"

// How a basic scaffolding directory looks like:
// my-extension
// ├── src
// │   └── MyExtension.php
// │   └── Resources
// │       └── config
// │           └── config.xml
// └── tests
//     └── TestBootstrap.php
// ├── composer.json
// ├── phpunit.xml
// ├── .gitignore

// twig tmpl.Delims("[[", "]]")
// $directory = \sprintf('%s/custom/%splugins/%s', $this->projectDir, $staticPrefix, $pluginName);

//go:embed stubs/*
var stubsFS embed.FS

// stubFuncs are helpers available inside the stub templates.
var stubFuncs = template.FuncMap{
	// jsonEscape makes a value safe inside a JSON string, e.g. the
	// backslashes of a PHP namespace: Swag\Example -> Swag\\Example.
	"jsonEscape": func(value string) (string, error) {
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("escape %q for json: %w", value, err)
		}

		// Drop the surrounding quotes json.Marshal adds.
		return string(encoded[1 : len(encoded)-1]), nil
	},
}

// createExtensionDir creates the directory for the new extension
func CreateExtensionDir(extensionDir string) error {
	// Make sure the extension directory does not already exist
	info, err := os.Stat(extensionDir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s exists and is not a directory", extensionDir)
		}
		return fmt.Errorf("extension directory already exists: %s", extensionDir)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat extension directory: %w", err)
	}

	// Create the directory with appropriate permissions
	err = os.Mkdir(extensionDir, 0o755) // Intended: if the parent directories (== Shopware Project) do not exist, this will fail.
	if err != nil {
		return fmt.Errorf("create extension directory: %w", err)
	}

	return nil
}

func CreateScaffoldingFiles(extensionDir, extensionName string) error {
	scaffoldingFiles := scaffoldingFiles(extensionName)
	for _, file := range scaffoldingFiles {
		
		data, err := createScaffoldingData(extensionName)
		if err != nil {
			return err
		}

		err = CreateExtensionFile(extensionDir, file, data)
		if err != nil {
			return err
		}
	
	}
	return nil
}

type ScaffoldingFile struct {
	Path    string
	StubPath string
}

// scaffoldingFiles returns a list of files with their paths and corresponding stub paths.
func scaffoldingFiles(extensionName string) []ScaffoldingFile {
	return []ScaffoldingFile{
		{
			Path:    "composer.json",
			StubPath: "stubs/composer.json.tmpl",
		},
		{
			Path:    "phpunit.xml",
			StubPath: "stubs/phpunit.xml.tmpl",
		},
		{
			Path:    "tests/TestBootstrap.php",
			StubPath: "stubs/test_bootstrap.php.tmpl",
		},
		{
			Path:    ".gitignore",
			StubPath: "stubs/gitignore.tmpl",
		},
		{
			Path:    "src/Resources/config/config.xml",
			StubPath: "stubs/config.xml.tmpl",
		},
		{
			Path:    filepath.Join("src", extensionName+".php"),
			StubPath: "stubs/plugin_class.php.tmpl",
		},
	}
}

// decided to not use temporary dir because there are only a few files
// assumes that the extension directory already exists and is empty.
func CreateExtensionFile(extensionDir string, file ScaffoldingFile, data scaffoldData) error {
	// create missing subdirectories under the extension directory
	dest := filepath.Join(extensionDir, file.Path)
	err := os.MkdirAll(filepath.Dir(dest), 0o755)
	if err != nil {
		return fmt.Errorf("create subdirectories: %w", err)
	}

	// create the file
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	
	// get scaffolding content
	stubBytes, err := stubsFS.ReadFile(file.StubPath)
	if err != nil {
		return fmt.Errorf("read stub file: %w", err)
	}

	// parse stub with data and write to file
	tmpl, err := template.New(file.Path).Funcs(stubFuncs).Parse(string(stubBytes))
	if err != nil {
		return fmt.Errorf("parse stub: %w", err)
	}

	err = tmpl.Execute(f, data)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}

	// flushing in-memory copy of recently written data to disk
	err = f.Sync()
	if err != nil {
		return fmt.Errorf("flush file to disk: %w", err)
	}

	return nil
}

type scaffoldData struct {
	TechnicalName string
	Namespace     string
	ClassName	 string
	ComposerName	string
}

func createScaffoldingData(extensionName string) (scaffoldData, error) {
	data := scaffoldData{}
	
	data.TechnicalName = extensionName
	data.Namespace = deriveNamespace(extensionName)
	data.ClassName = deriveClassName(extensionName)
	data.ComposerName = deriveComposerName(extensionName)

	return data, nil
}

// deriveNamespace turns a technical plugin name into a PHP namespace.
// The first PascalCase word is the vendor prefix, the rest stay one segment:
// SwagBasicExample → Swag\BasicExample.
func deriveNamespace(extensionName string) string {
	parts := splitPascalCase(extensionName)
	if len(parts) < 2 {
		return extensionName
	}

	return parts[0] + "\\" + strings.Join(parts[1:], "")
}

// deriveClassName is the PHP plugin class, which matches the technical name:
// SwagBasicExample → SwagBasicExample.
func deriveClassName(extensionName string) string {
	return extensionName
}

// deriveComposerName turns a technical plugin name into a Composer package name:
// SwagBasicExample → swag/basic-example.
func deriveComposerName(extensionName string) string {
	parts := splitPascalCase(extensionName)
	if len(parts) == 0 {
		return ""
	}

	vendor := strings.ToLower(parts[0])
	if len(parts) == 1 {
		return vendor + "/" + vendor
	}

	return vendor + "/" + strings.ToLower(strings.Join(parts[1:], "-"))
}

// splitPascalCase is a helper function and splits a PascalCase string into its constituent words.
func splitPascalCase(name string) []string {
	if name == "" {
		return nil
	}

	runes := []rune(name)
	start := 0
	parts := make([]string, 0, 4)

	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) {
			parts = append(parts, string(runes[start:i]))
			start = i
		}
	}

	return append(parts, string(runes[start:]))
}

// RemoveCreatedExtensionDir deletes the directory created by CreateExtensionDir.
// It only removes a path that is an extension folder (custom/plugins/<name> or
// custom/static-plugins/<name>), never parents, the project root, or a symlink.
func RemoveCreatedExtensionDir(extensionDir string) error {
	// Reject an empty path variable.
	if strings.TrimSpace(extensionDir) == "" {
		return errors.New("extension directory variable must not be empty")
	}

	// Turn the path into an absolute, cleaned path (no "..").
	abs, err := filepath.Abs(extensionDir)
	if err != nil {
		return fmt.Errorf("resolve extension directory: %w", err)
	}
	abs = filepath.Clean(abs)

	// Never delete the filesystem root.
	if abs == string(filepath.Separator) {
		return fmt.Errorf("refusing to remove %s", abs)
	}

	// The last segment must be a real folder name.
	name := filepath.Base(abs)
	if name == "." || name == ".." || name == string(filepath.Separator) {
		return fmt.Errorf("refusing to remove %s", abs)
	}

	// Parent must be custom/plugins or custom/static-plugins.
	parent := filepath.Dir(abs)
	pluginRoot := filepath.Join(filepath.Base(filepath.Dir(parent)), filepath.Base(parent))
	if pluginRoot != filepath.FromSlash(store) && pluginRoot != filepath.FromSlash(private) {
		return fmt.Errorf("refusing to remove %s: not an extension directory", abs)
	}

	// Inspect the path itself, do not follow a symlink.
	info, err := os.Lstat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // Already gone.
		}
		return fmt.Errorf("stat extension directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to remove %s: is a symlink", abs)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", abs)
	}

	// Delete the folder and everything inside it.
	if err := os.RemoveAll(abs); err != nil {
		return fmt.Errorf("remove extension directory: %w", err)
	}

	return nil
}

func RequireDirExists(path string) error {
	return nil
}

/* how the service.php gets generated
intro
    + CommandGenerator snippet?        // if --create-command
    + StorefrontController snippet?
    + StoreApiRoute snippet?
    + EntityGenerator snippet?         // once per entity
    + EventSubscriber snippet?
    + ScheduledTask snippet?
outro
*/
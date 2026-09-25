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

const (
	privatePluginRoot = "custom/static-plugins"
	storePluginRoot   = "custom/plugins"
	composerNameRegex = "^[a-z0-9]([_.-]?[a-z0-9]+)*/[a-z0-9](([_.]?|-{0,2})[a-z0-9]+)*$"
	packageNameRegex  = "^[a-z0-9]([_.-]?[a-z0-9]+)*/[a-z0-9](([_.]|-{1,2})?[a-z0-9]+)*$"
)

//go:embed stubs/*
var stubsFS embed.FS

// stubFuncs are helpers available inside the stub templates.
var stubFuncs = template.FuncMap{
	// jsonEscape makes a value safe inside a JSON string
	"jsonEscape": func(value string) (string, error) {
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("escape %q for json: %w", value, err)
		}

		// Drop the surrounding quotes json.Marshal adds.
		return string(encoded[1 : len(encoded)-1]), nil
	},
	// escapeBackslash makes a value safe inside PHP strings by escaping backslashes.
	"escapeBackslash": func(value string) string {
		return strings.ReplaceAll(value, "\\", "\\\\")
	},
}

type scaffoldingFile struct {
	Path     string
	StubPath string
}

// pluginScaffoldingFiles returns the files in a platform plugin scaffold.
func pluginScaffoldingFiles(className string) []scaffoldingFile {
	return []scaffoldingFile{
		{
			Path:     "composer.json",
			StubPath: "stubs/composer.json.tmpl",
		},
		{
			Path:     "phpunit.xml",
			StubPath: "stubs/phpunit.xml.tmpl",
		},
		{
			Path:     "tests/TestBootstrap.php",
			StubPath: "stubs/test_bootstrap.php.tmpl",
		},
		{
			Path:     ".gitignore",
			StubPath: "stubs/gitignore.tmpl",
		},
		{
			Path:     filepath.Join("src", className+".php"),
			StubPath: "stubs/plugin_class.php.tmpl",
		},
	}
}

// themeScaffoldingFiles returns the default files generated for a storefront theme.
func themeScaffoldingFiles(data scaffoldData) []scaffoldingFile {
	return []scaffoldingFile{
		{
			Path:     "composer.json",
			StubPath: "stubs/theme/theme_composer.json.tmpl",
		},
		{
			Path:     filepath.Join("src", data.ClassName+".php"),
			StubPath: "stubs/theme/theme_class.php.tmpl",
		},
		{
			Path:     "src/Resources/theme.json",
			StubPath: "stubs/theme/theme.json.tmpl",
		},
		{
			Path:     "src/Resources/app/storefront/src/scss/overrides.scss",
			StubPath: "stubs/theme/theme_overrides.scss.tmpl",
		},
		{
			Path: "src/Resources/app/storefront/src/scss/base.scss",
		},
		{
			Path: "src/Resources/app/storefront/src/assets/.gitkeep",
		},
		{
			Path: "src/Resources/app/storefront/src/main.js",
		},
		{
			Path: filepath.Join(
				"src/Resources/app/storefront/dist/storefront/js",
				data.AssetName,
				data.AssetName+".js",
			),
		},
	}
}

// CreateExtensionDir creates an empty extension directory. Its parents must already exist.
func CreateExtensionDir(extensionDir string) error {
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

	parent := filepath.Dir(extensionDir)
	info, err = os.Stat(parent)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("extension parent directory does not exist: %s", parent)
	}
	if err != nil {
		return fmt.Errorf("stat extension parent directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("extension parent path is not a directory: %s", parent)
	}

	if err := os.Mkdir(extensionDir, 0o755); err != nil {
		return fmt.Errorf("create extension directory: %w", err)
	}

	return nil
}

// CreatePluginFiles creates the files for a platform plugin.
func CreatePluginFiles(extensionDir, extensionName, vendorName string) error {
	data := createScaffoldingData(vendorName, extensionName)
	return createExtensionFiles(extensionDir, pluginScaffoldingFiles(data.ClassName), data)
}

// CreateThemeFiles creates the files for a storefront theme.
func CreateThemeFiles(extensionDir, extensionName, vendorName string) error {
	data := createScaffoldingData(vendorName, extensionName)
	return createExtensionFiles(extensionDir, themeScaffoldingFiles(data), data)
}

func createExtensionFiles(extensionDir string, files []scaffoldingFile, data scaffoldData) error {
	for _, file := range files {
		err := createFileWithScaffolding(extensionDir, file, data)
		if err != nil {
			return err
		}
	}

	return nil
}

// createFileWithScaffolding renders an embedded template or creates an empty placeholder.
func createFileWithScaffolding(extensionDir string, file scaffoldingFile, data scaffoldData) (err error) {
	dest := filepath.Join(extensionDir, file.Path)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create subdirectories: %w", err)
	}

	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close file: %w", closeErr)
		}
	}()

	if file.StubPath != "" {
		stubBytes, err := stubsFS.ReadFile(file.StubPath)
		if err != nil {
			return fmt.Errorf("read stub file: %w", err)
		}

		tmpl, err := template.New(file.Path).Funcs(stubFuncs).Parse(string(stubBytes))
		if err != nil {
			return fmt.Errorf("parse stub: %w", err)
		}

		if err := tmpl.Execute(f, data); err != nil {
			return fmt.Errorf("render: %w", err)
		}
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("flush file to disk: %w", err)
	}

	return nil
}

type scaffoldData struct {
	Namespace    string
	ClassName    string
	ComposerName string
	AssetName    string
}

func createScaffoldingData(vendorName string, extensionName string) scaffoldData {
	className := DeriveClassName(vendorName, extensionName)

	return scaffoldData{
		Namespace:    DeriveNamespace(vendorName, extensionName),
		ClassName:    className,
		ComposerName: DeriveComposerName(vendorName, extensionName),
		AssetName:    DeriveAssetName(className),
	}
}

// DeriveNamespace turns a given extension name and vendor name into a PHP namespace.
func DeriveNamespace(vendorName string, extensionName string) string {
	if vendorName == "" {
		return extensionName
	}
	return vendorName + "\\" + extensionName
}

// DeriveComposerName turns a given extension name and vendor name into a valid Composer package name:
// Vendor, BasicExample → vendor/basic-example.
func DeriveComposerName(vendor string, name string) string {
	vendorParts := splitPascalCase(vendor)
	nameParts := splitPascalCase(name)

	lowerVendor := strings.ToLower(strings.Join(vendorParts, "-"))
	lowerName := strings.ToLower(strings.Join(nameParts, "-"))

	if lowerVendor == "" {
		lowerVendor = lowerName
	}

	composerName := lowerVendor + "/" + lowerName

	return composerName
}

// DeriveClassName turns a given extension name and vendor name into a valid PHP class name.
func DeriveClassName(vendorName string, extensionName string) string {
	return vendorName + extensionName
}

func DeriveAssetName(className string) string {
	return strings.ToLower(strings.Join(splitPascalCase(className), "-"))
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
	if err := validateRemovableExtensionDir(extensionDir); err != nil {
		return err
	}
	abs, _ := filepath.Abs(extensionDir) // Already validated, so error can be ignored.

	// Delete the folder and everything inside it.
	if err := os.RemoveAll(abs); err != nil {
		return fmt.Errorf("remove extension directory: %w", err)
	}

	return nil
}

func validateRemovableExtensionDir(extensionDir string) error {
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
	if pluginRoot != filepath.FromSlash(storePluginRoot) && pluginRoot != filepath.FromSlash(privatePluginRoot) {
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
	return nil
}

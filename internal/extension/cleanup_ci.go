package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dario.cat/mergo"
	"golang.org/x/text/language"

	"github.com/shopware/shopware-cli/logging"
)

// CleanupAdministrationFiles merges snippet files, deletes the admin source folder,
// and recreates it with only the merged snippets and an empty main.js.
func CleanupAdministrationFiles(ctx context.Context, folder string) error {
	adminFolder := filepath.Join(folder, "Resources", "app", "administration")

	if _, err := os.Stat(adminFolder); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	logging.FromContext(ctx).Infof("Merging Administration snippet for %s", folder)

	snippetFiles := make(map[string][]string)

	err := filepath.WalkDir(adminFolder, func(path string, d os.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		fileExt := filepath.Ext(path)

		if fileExt != ".json" {
			return nil
		}

		languageName := strings.TrimSuffix(filepath.Base(path), fileExt)

		if _, err := language.Parse(languageName); err != nil {
			logging.FromContext(ctx).Infof("Ignoring invalid locale filename %s", path)
			return nil //nolint:nilerr
		}

		if language.Make(languageName).IsRoot() {
			return nil
		}

		if _, ok := snippetFiles[languageName]; !ok {
			snippetFiles[languageName] = []string{}
		}

		snippetFiles[languageName] = append(snippetFiles[languageName], path)

		return nil
	})
	if err != nil {
		return err
	}

	for language, files := range snippetFiles {
		if len(files) == 1 {
			data, err := os.ReadFile(files[0])
			if err != nil {
				return err
			}

			if err := os.WriteFile(filepath.Join(folder, language), data, 0o644); err != nil {
				return err
			}

			continue
		}

		merged := make(map[string]interface{})

		for _, file := range files {
			snippetFile := make(map[string]interface{})

			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}

			if err := json.Unmarshal(data, &snippetFile); err != nil {
				return fmt.Errorf("unable to parse %s: %w", file, err)
			}

			if err := mergo.Merge(&merged, snippetFile, mergo.WithOverride); err != nil {
				return err
			}
		}

		mergedData, err := json.Marshal(merged)
		if err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(folder, language), mergedData, 0o644); err != nil {
			return err
		}
	}

	logging.FromContext(ctx).Infof("Deleting Administration source files for %s", folder)

	if err := os.RemoveAll(adminFolder); err != nil {
		return err
	}

	logging.FromContext(ctx).Infof("Migrating generated snippet file for %s", folder)

	snippetFolder := filepath.Join(adminFolder, "src", "app", "snippet")
	if err := os.MkdirAll(snippetFolder, 0o755); err != nil {
		return err
	}

	for language := range snippetFiles {
		if err := os.Rename(filepath.Join(folder, language), filepath.Join(snippetFolder, language+".json")); err != nil {
			return err
		}
	}

	logging.FromContext(ctx).Infof("Creating empty main.js for %s", folder)
	if err := os.WriteFile(filepath.Join(adminFolder, "src", "main.js"), []byte(""), 0o644); err != nil {
		return err
	}

	return nil
}

// CleanupJavaScriptSourceMaps removes .js.map files and their corresponding
// sourceMappingURL comments from .js files.
func CleanupJavaScriptSourceMaps(folder string) error {
	if _, err := os.Stat(folder); err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	return filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".js.map") {
			return nil
		}

		if err := os.Remove(path); err != nil {
			return err
		}

		expectedJsFile := path[0 : len(path)-4]

		if _, err := os.Stat(expectedJsFile); err != nil {
			if os.IsNotExist(err) {
				return nil
			}

			return err
		}

		content, readErr := os.ReadFile(expectedJsFile)
		if readErr != nil {
			return fmt.Errorf("could not open file %s: %w", expectedJsFile, readErr)
		}

		expectedSourceMapComment := fmt.Sprintf("//# sourceMappingURL=%s", filepath.Base(path))

		overwrittenContent := strings.ReplaceAll(string(content), expectedSourceMapComment, "")

		return os.WriteFile(expectedJsFile, []byte(overwrittenContent), 0o644)
	})
}

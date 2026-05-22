package envfile

import (
	"bufio"
	"bytes"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joho/godotenv"
)

// LoadSymfonyEnvFile loads the Symfony .env file from the project root.
func LoadSymfonyEnvFile(projectRoot string) error {
	currentEnv := os.Getenv("APP_ENV")
	if currentEnv == "" {
		currentEnv = "dev"
	}

	possibleEnvFiles := []string{
		path.Join(projectRoot, ".env.dist"),
		path.Join(projectRoot, ".env"),
		path.Join(projectRoot, ".env.local"),
		path.Join(projectRoot, ".env."+currentEnv),
		path.Join(projectRoot, ".env."+currentEnv+".local"),
	}

	var foundEnvFiles []string

	for _, envFile := range possibleEnvFiles {
		if _, err := os.Stat(envFile); err == nil {
			foundEnvFiles = append(foundEnvFiles, envFile)
		}
	}

	if len(foundEnvFiles) == 0 {
		return nil
	}

	currentMap, err := godotenv.Read(foundEnvFiles...)
	if err != nil {
		return err
	}

	for key, value := range currentMap {
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}

	return nil
}

// ReadValue returns the effective value of key in the project's Symfony env
// files. Precedence follows Symfony: .env.dist < .env < .env.local.
// An empty string is returned when the key is undefined or no env file exists.
func ReadValue(projectRoot, key string) (string, error) {
	values, err := ReadValues(projectRoot, key)
	if err != nil {
		return "", err
	}
	return values[key], nil
}

// ReadValues returns the resolved values for the requested keys. The returned
// map always contains every requested key; missing keys map to "".
func ReadValues(projectRoot string, keys ...string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = ""
	}

	files := resolveEnvFiles(projectRoot)
	if len(files) == 0 {
		return out, nil
	}

	values, err := godotenv.Read(files...)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		out[k] = values[k]
	}
	return out, nil
}

// WriteValue is a convenience wrapper around WriteValues for a single key.
func WriteValue(projectRoot, key, value string) error {
	return WriteValues(projectRoot, map[string]string{key: value})
}

// WriteValues persists all entries into .env.local. Existing assignments are
// replaced in place to preserve surrounding lines, comments and formatting;
// missing keys are appended in deterministic (sorted) order. The file is
// created when it does not exist.
func WriteValues(projectRoot string, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}

	envLocalPath := filepath.Join(projectRoot, ".env.local")

	existing, err := os.ReadFile(envLocalPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	updated := existing
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := values[key]
		var replaced bool
		updated, replaced = replaceEnvLine(updated, key, value)
		if !replaced {
			if len(updated) > 0 && !bytes.HasSuffix(updated, []byte("\n")) {
				updated = append(updated, '\n')
			}
			updated = append(updated, []byte(key+"="+value+"\n")...)
		}
	}

	return os.WriteFile(envLocalPath, updated, 0o644)
}

func resolveEnvFiles(projectRoot string) []string {
	candidates := []string{
		filepath.Join(projectRoot, ".env.dist"),
		filepath.Join(projectRoot, ".env"),
		filepath.Join(projectRoot, ".env.local"),
	}
	var found []string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			found = append(found, p)
		}
	}
	return found
}

// replaceEnvLine returns content with the first assignment of key replaced
// by `key=value`. The second return value reports whether a replacement
// happened. Comments and unrelated lines are preserved verbatim.
func replaceEnvLine(content []byte, key, value string) ([]byte, bool) {
	var out bytes.Buffer
	replaced := false

	scanner := bufio.NewScanner(bytes.NewReader(content))
	hadTrailingNewline := bytes.HasSuffix(content, []byte("\n"))

	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if !first {
			out.WriteByte('\n')
		}
		first = false

		trimmed := strings.TrimLeft(line, " \t")
		if !replaced && strings.HasPrefix(trimmed, key+"=") {
			out.WriteString(key + "=" + value)
			replaced = true
			continue
		}
		out.WriteString(line)
	}

	if hadTrailingNewline {
		out.WriteByte('\n')
	}

	return out.Bytes(), replaced
}

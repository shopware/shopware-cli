package envfile

import (
	"bufio"
	"bytes"
	"os"
	"path"
	"path/filepath"
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

// ReadAppEnv resolves the effective APP_ENV value for the project at
// projectRoot by reading the Symfony env files in the conventional order.
// Returns an empty string when APP_ENV is not defined in any file.
func ReadAppEnv(projectRoot string) (string, error) {
	possibleEnvFiles := []string{
		filepath.Join(projectRoot, ".env.dist"),
		filepath.Join(projectRoot, ".env"),
		filepath.Join(projectRoot, ".env.local"),
	}

	var foundEnvFiles []string
	for _, envFile := range possibleEnvFiles {
		if _, err := os.Stat(envFile); err == nil {
			foundEnvFiles = append(foundEnvFiles, envFile)
		}
	}

	if len(foundEnvFiles) == 0 {
		return "", nil
	}

	currentMap, err := godotenv.Read(foundEnvFiles...)
	if err != nil {
		return "", err
	}

	return currentMap["APP_ENV"], nil
}

// WriteAppEnv sets APP_ENV in .env.local. If the key already exists, its
// value is replaced in place to preserve surrounding lines, comments and
// formatting. Otherwise the assignment is appended to the file. The file is
// created when missing.
func WriteAppEnv(projectRoot, value string) error {
	envLocalPath := filepath.Join(projectRoot, ".env.local")

	existing, err := os.ReadFile(envLocalPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	updated, replaced := replaceEnvLine(existing, "APP_ENV", value)
	if !replaced {
		if len(updated) > 0 && !bytes.HasSuffix(updated, []byte("\n")) {
			updated = append(updated, '\n')
		}
		updated = append(updated, []byte("APP_ENV="+value+"\n")...)
	}

	return os.WriteFile(envLocalPath, updated, 0o644)
}

// replaceEnvLine returns content with the first assignment of key replaced
// by `key=value`. The second return value reports whether a replacement
// happened. Comments and unrelated lines are preserved verbatim.
func replaceEnvLine(content []byte, key, value string) ([]byte, bool) {
	var out bytes.Buffer
	replaced := false

	scanner := bufio.NewScanner(bytes.NewReader(content))
	// Preserve trailing newline if present
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

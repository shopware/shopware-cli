package symfonyconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// loadDotenv mirrors Symfony's dotenv loading order:
//
//	.env             (committed defaults)
//	.env.local       (local overrides, not committed)
//	.env.<env>       (env-specific defaults)
//	.env.<env>.local (env-specific local overrides)
//
// Later files override earlier ones. Process env (os.Getenv) wins over all
// dotenv values to match Symfony's behavior where the real environment is
// authoritative.
func loadDotenv(projectRoot, env string) (map[string]string, error) {
	out := map[string]string{}

	candidates := []string{
		".env",
		".env.local",
		".env." + env,
		".env." + env + ".local",
	}

	for _, name := range candidates {
		path := filepath.Join(projectRoot, name)
		if !fileExists(path) {
			continue
		}
		m, err := godotenv.Read(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		for k, v := range m {
			out[k] = v
		}
	}

	// Process env wins over dotenv files. Empty values are skipped so that an
	// inherited empty variable from the shell or test runner doesn't shadow a
	// real dotenv value - this matches what users intuitively expect.
	for _, kv := range os.Environ() {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				name := kv[:i]
				value := kv[i+1:]
				if value == "" {
					continue
				}
				out[name] = value
				break
			}
		}
	}

	return out, nil
}

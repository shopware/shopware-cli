package shop

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode"
)

type EnvironmentSSHSharedConfig struct {
	// Project-relative files to share between releases. Omit to use .env.local, install.lock, public/.htaccess and public/.user.ini. An explicit list replaces the defaults; [] disables shared files.
	Files *[]string `yaml:"files,omitempty"`
	// Project-relative directories to share between releases. Omit to use config/jwt, files, var/log, public/media, public/plugins, public/thumbnail, public/sitemap and public/theme. An explicit list replaces the defaults; [] disables shared directories.
	Directories *[]string `yaml:"directories,omitempty"`
}

// SharedPaths resolves each list independently. Pointers preserve explicitly
// empty lists when project configuration is written back to YAML.
func (c *EnvironmentSSHConfig) SharedPaths() (files, directories []string, err error) {
	files = []string{".env.local", "install.lock", "public/.htaccess", "public/.user.ini"}
	directories = []string{"config/jwt", "files", "var/log", "public/media", "public/plugins", "public/thumbnail", "public/sitemap", "public/theme"}
	if c != nil && c.Shared != nil {
		if c.Shared.Files != nil {
			files = slices.Clone(*c.Shared.Files)
		}
		if c.Shared.Directories != nil {
			directories = slices.Clone(*c.Shared.Directories)
		}
	}
	paths := slices.Concat(files, directories)
	for i, name := range paths {
		// These are remote POSIX paths, independent of the client's OS. Require
		// canonical paths so aliases cannot bypass duplicate/overlap checks.
		if name == "" || name == "." || name == ".." || path.IsAbs(name) ||
			path.Clean(name) != name || strings.HasPrefix(name, "../") ||
			strings.ContainsRune(name, '\\') || strings.ContainsFunc(name, unicode.IsControl) {
			return nil, nil, fmt.Errorf("ssh.shared path %q must be a clean project-relative path", name)
		}
		for _, previous := range paths[:i] {
			if name == previous || strings.HasPrefix(name, previous+"/") || strings.HasPrefix(previous, name+"/") {
				return nil, nil, fmt.Errorf("ssh.shared paths %q and %q overlap", previous, name)
			}
		}
	}
	return files, directories, nil
}

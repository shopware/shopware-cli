package tracking

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/shopware/shopware-cli/logging"
)

const (
	defaultTrackingDomain = "udp.usage.shopware.io"
	defaultTrackingPort   = "9000"
	eventPrefix           = "shopware_cli."
	idFileName            = ".shopware-cli-id"
)

type event struct {
	Event     string            `json:"event"`
	Tags      map[string]string `json:"tags"`
	UserID    string            `json:"user_id"`
	Timestamp string            `json:"timestamp"`
}

var (
	id   string
	addr string
)

func init() {
	domain := os.Getenv("SHOPWARE_TRACKING_DOMAIN")
	if domain == "" {
		domain = defaultTrackingDomain
	}
	addr = net.JoinHostPort(domain, defaultTrackingPort)

	id = resolveID()
}

// resolveID returns a stable user ID. On CI systems it derives a deterministic
// ID from environment variables so the same repository always maps to the same
// tracking identity. On local machines it persists a random ID to the user
// config directory.
func resolveID() string {
	// CI systems: derive a deterministic ID from repository identifiers.
	// Use values that are globally unique across instances:
	// - GITHUB_REPOSITORY: "owner/repo" (unique within github.com)
	// - CI_PROJECT_URL: "https://gitlab.example.com/group/repo" (includes hostname, unique across GitLab instances)
	// - BITBUCKET_REPO_FULL_NAME: "workspace/repo" (unique within bitbucket.org)
	ciEnvVars := []string{
		"GITHUB_REPOSITORY",
		"CI_PROJECT_URL",
		"BITBUCKET_REPO_FULL_NAME",
	}
	for _, envVar := range ciEnvVars {
		if v := os.Getenv(envVar); v != "" {
			h := sha256.Sum256([]byte(v))
			return hex.EncodeToString(h[:16])
		}
	}

	// Local: try to load a persisted ID from the config directory.
	if configDir, err := os.UserConfigDir(); err == nil {
		idPath := filepath.Join(configDir, idFileName)

		if data, err := os.ReadFile(idPath); err == nil && len(data) == 32 {
			return string(data)
		}

		// Generate and persist a new random ID.
		b := make([]byte, 16)
		if _, err := rand.Read(b); err == nil {
			newID := hex.EncodeToString(b)
			_ = os.WriteFile(idPath, []byte(newID), 0o600)
			return newID
		}
	}

	// Fallback: generate an ephemeral random ID.
	b := make([]byte, 16)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}
	return ""
}

func Track(ctx context.Context, eventName string, tags map[string]string) {
	if _, ok := os.LookupEnv("DO_NOT_TRACK"); ok {
		return
	}

	if tags == nil {
		tags = make(map[string]string)
	}

	e := event{
		Event:     eventPrefix + eventName,
		Tags:      tags,
		UserID:    id,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	payload, err := json.Marshal(e)
	if err != nil {
		logging.FromContext(ctx).Debugf("tracking: failed to marshal event: %v", err)
		return
	}

	conn, err := net.Dial("udp", addr)
	if err != nil {
		logging.FromContext(ctx).Debugf("tracking: failed to connect: %v", err)
		return
	}
	defer conn.Close()

	if _, err := conn.Write(payload); err != nil {
		logging.FromContext(ctx).Debugf("tracking: failed to send event: %v", err)
	}
}

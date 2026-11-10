// Package deprecation reports use of deprecated commands, flags and config
// keys as described in docs/adr/2026-11-09-deprecation-policy.md.
package deprecation

import (
	"context"
	"slices"
	"sync"

	"github.com/shopware/shopware-cli/logging"
)

var (
	usedMu sync.Mutex
	used   []string
)

// Message is the sentence cobra appends to its own "has been deprecated" or
// "is deprecated" prefix for flags and commands. replacement may be empty.
func Message(removedBy, replacement string) string {
	message := "it will be removed by " + removedBy
	if replacement != "" {
		message += ", use " + replacement + " as replacement"
	}

	return message
}

// Warn logs that subject is deprecated and reports id via telemetry.
// replacement may be empty.
func Warn(ctx context.Context, id, subject, removedBy, replacement string) {
	message := subject + " is deprecated and will be removed by " + removedBy
	if replacement != "" {
		message += ", use " + replacement + " as replacement"
	}
	logging.FromContext(ctx).Warn(message)
	Track(id)
}

// Track records that the run used the deprecated item id.
func Track(id string) {
	usedMu.Lock()
	defer usedMu.Unlock()

	if !slices.Contains(used, id) {
		used = append(used, id)
	}
}

// Used returns the ids recorded with Track, sorted.
func Used() []string {
	usedMu.Lock()
	defer usedMu.Unlock()

	return slices.Sorted(slices.Values(used))
}

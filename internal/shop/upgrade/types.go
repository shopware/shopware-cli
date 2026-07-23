// Package upgrade implements the backend for the interactive Shopware upgrade
// wizard (`shopware-cli project upgrade`): project readiness checks, the
// upgrade version catalog, extension compatibility classification, and the
// execution engine that applies the upgrade locally. It contains no UI code so
// every part can be tested without a terminal.
package upgrade

import (
	"time"

	"github.com/shyim/go-version"
)

// CheckState describes the lifecycle of a single check or execution step.
type CheckState int

const (
	StatePending CheckState = iota
	StateRunning
	StateOK
	StateWarn
	StateFail
)

// ReadinessCheck is one row of the "Check project readiness" panel.
type ReadinessCheck struct {
	ID       string
	Label    string
	Value    string // short right-aligned value, e.g. "yes" or "14"
	Detail   string // dimmed sub-line, e.g. the repository path
	State    CheckState
	Blocking bool // a failed blocking check prevents continuing
}

// Failed reports whether the check blocks the wizard from continuing.
func (c ReadinessCheck) Failed() bool {
	return c.Blocking && c.State == StateFail
}

// InstalledExtension is an extension found in the project, either managed
// through Composer (vendor/) or locally in custom/plugins and friends.
type InstalledExtension struct {
	Name            string // technical name, e.g. SwagPayPal
	Package         string // composer package name, e.g. swag/paypal
	Path            string
	Version         string
	ComposerManaged bool
}

// Readiness is the result of RunReadinessChecks.
type Readiness struct {
	Checks         []ReadinessCheck
	CurrentVersion *version.Version
	Extensions     []InstalledExtension
}

// Blocked reports whether any blocking readiness check failed.
func (r Readiness) Blocked() bool {
	for _, c := range r.Checks {
		if c.Failed() {
			return true
		}
	}
	return false
}

// ComposerManagedCount returns how many discovered extensions are managed via Composer.
func (r Readiness) ComposerManagedCount() int {
	n := 0
	for _, e := range r.Extensions {
		if e.ComposerManaged {
			n++
		}
	}
	return n
}

// VersionOption is one selectable target version in the version picker.
type VersionOption struct {
	Version         *version.Version
	Tag             string // e.g. "recommended", "latest 6.6 patch"
	SupportType     string // "active", "security", "eol" or "" when unknown
	SupportUntil    time.Time
	ReleaseNotesURL string
}

// SupportLeft returns a compact human description of the remaining support
// window, e.g. "1y 7m", or "" when unknown.
func (o VersionOption) SupportLeft() string {
	return supportLeft(o.SupportUntil, time.Now())
}

// Catalog lists the versions the project can upgrade to, newest first.
type Catalog struct {
	Current     *version.Version
	Options     []VersionOption
	Recommended int // index into Options, -1 when empty
	LatestPatch int // index of the newest patch of the current minor, -1 when none
}

// ExtStatus classifies an extension's compatibility with the target version.
type ExtStatus int

const (
	// ExtOK: the installed release already allows the target version.
	ExtOK ExtStatus = iota
	// ExtNeedsUpdate: a compatible Composer release exists but is newer than
	// the installed one.
	ExtNeedsUpdate
	// ExtMismatch: the Store labels the extension compatible but no Composer
	// release satisfies the target — Composer is the source of truth.
	ExtMismatch
	// ExtDeprecated: the extension is deprecated/replaced per Store metadata.
	ExtDeprecated
	// ExtBlocked: no compatible Composer release exists for the target.
	ExtBlocked
	// ExtReview: a local (non Composer-managed) extension the wizard does not
	// check; needs a manual review.
	ExtReview
)

// Blocksupgrade reports whether this status prevents starting the upgrade.
func (s ExtStatus) BlocksUpgrade() bool {
	return s == ExtBlocked || s == ExtMismatch
}

// Rank orders statuses by severity for the extension queue (most severe first).
func (s ExtStatus) Rank() int {
	switch s {
	case ExtBlocked:
		return 0
	case ExtMismatch:
		return 1
	case ExtDeprecated:
		return 2
	case ExtNeedsUpdate:
		return 3
	case ExtReview:
		return 4
	case ExtOK:
		return 5
	}
	return 6
}

// Label returns the short result column text used in the extension queue.
func (s ExtStatus) Label() string {
	switch s {
	case ExtOK:
		return "ok"
	case ExtNeedsUpdate:
		return "needs update"
	case ExtMismatch:
		return "mismatch"
	case ExtDeprecated:
		return "replace"
	case ExtBlocked:
		return "blocked"
	case ExtReview:
		return "review"
	}
	return "unknown"
}

// ExtensionResult is the classified compatibility outcome for one extension.
type ExtensionResult struct {
	Extension InstalledExtension
	Status    ExtStatus
	// Available is the lowest extension release compatible with the target
	// version ("" when none exists).
	Available string
	// StoreLabel is the compatibility label reported by the Shopware Store
	// ("" when the extension is unknown to the Store).
	StoreLabel string
	// Replacement names a successor extension when Status is ExtDeprecated.
	Replacement string
	// ChangelogURL points at the release notes of the providing repository.
	ChangelogURL string
	// Detail is a one-line explanation shown in the detail overlay.
	Detail string
}

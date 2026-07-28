package pluginmigrate

import (
	"fmt"
	"strings"

	"github.com/shyim/go-version"
)

// ActionKind classifies how one extension is migrated.
type ActionKind int

const (
	// ActionStoreRequire requires the extension from the Shopware Store
	// packagist and removes the local copy.
	ActionStoreRequire ActionKind = iota
	// ActionComposerRequire requires the extension from a repository the
	// project already resolves against (packagist.org or a configured
	// composer repository) and removes the local copy. Needs no Store token.
	ActionComposerRequire
	// ActionPathRepository keeps the files where they are and registers them
	// as a Composer path repository.
	ActionPathRepository
	// ActionUnsupported marks an extension the migrator cannot handle (no
	// composer.json with a package name); it is left untouched.
	ActionUnsupported
)

// Label returns the review-table text for an action.
func (k ActionKind) Label() string {
	switch k {
	case ActionStoreRequire:
		return "require from Shopware Store"
	case ActionComposerRequire:
		return "require from Packagist"
	case ActionPathRepository:
		return "manage via path repository"
	case ActionUnsupported:
		return "skipped (no composer.json name)"
	}
	return "unknown"
}

// PlannedExtension is one extension with its migration action.
type PlannedExtension struct {
	ScannedExtension
	Kind ActionKind
	// RequireArg is the `composer require` argument, empty for unsupported
	// extensions.
	RequireArg string
}

// Plan is the full migration the review step confirms.
type Plan struct {
	Extensions []PlannedExtension
	// AddStoreRepository adds packages.shopware.com to composer.json.
	AddStoreRepository bool
}

// BuildPlan classifies the scanned extensions, preferring the source that
// keeps updates flowing: the Shopware Store (token required), then a
// repository the project already resolves against (packagist.org or a
// configured composer repository — no token needed) when it offers the exact
// installed version, then a path repository as the fallback that also puts
// the extension under Composer management — just without updates.
func BuildPlan(extensions []ScannedExtension, avail Availability) Plan {
	var plan Plan

	for _, ext := range extensions {
		planned := PlannedExtension{ScannedExtension: ext}

		storeName := "store.shopware.com/" + strings.ToLower(ext.Name)
		_, inStore := avail.Store[storeName]

		switch {
		case inStore && ext.Version != "":
			planned.Kind = ActionStoreRequire
			planned.RequireArg = fmt.Sprintf("%s:%s", storeName, ext.Version)
			plan.AddStoreRepository = true
		case ext.ComposerName != "" && versionPublished(avail.Published[ext.ComposerName], ext.Version):
			planned.Kind = ActionComposerRequire
			planned.RequireArg = fmt.Sprintf("%s:%s", ext.ComposerName, ext.Version)
		case ext.ComposerName != "":
			planned.Kind = ActionPathRepository
			planned.RequireArg = ext.ComposerName + ":*"
		default:
			planned.Kind = ActionUnsupported
		}

		plan.Extensions = append(plan.Extensions, planned)
	}

	return plan
}

// versionPublished reports whether the locally installed version is among the
// published ones. Only an exact match qualifies — requiring a different
// release than the code running in the shop is not a safe migration; such
// extensions fall back to a path repository instead.
func versionPublished(published []string, local string) bool {
	if local == "" {
		return false
	}

	localVersion, err := version.NewVersion(local)
	for _, candidate := range published {
		if candidate == local {
			return true
		}
		if err != nil {
			continue
		}
		if parsed, parseErr := version.NewVersion(candidate); parseErr == nil && parsed.Equal(localVersion) {
			return true
		}
	}
	return false
}

// Count returns how many extensions carry the given action.
func (p Plan) Count(kind ActionKind) int {
	n := 0
	for _, ext := range p.Extensions {
		if ext.Kind == kind {
			n++
		}
	}
	return n
}

// RequireArgs lists the `composer require` arguments of the plan.
func (p Plan) RequireArgs() []string {
	var args []string
	for _, ext := range p.Extensions {
		if ext.RequireArg != "" {
			args = append(args, ext.RequireArg)
		}
	}
	return args
}

// PathRepositories lists the relative paths registered as path repositories.
func (p Plan) PathRepositories() []string {
	var paths []string
	for _, ext := range p.Extensions {
		if ext.Kind == ActionPathRepository {
			paths = append(paths, ext.RelPath)
		}
	}
	return paths
}

// RemoveDirs lists the absolute directories removed after the require
// succeeded — the local copies of extensions that are now installed from a
// repository instead.
func (p Plan) RemoveDirs() []string {
	var dirs []string
	for _, ext := range p.Extensions {
		if ext.Kind == ActionStoreRequire || ext.Kind == ActionComposerRequire {
			dirs = append(dirs, ext.Path)
		}
	}
	return dirs
}

// Actionable reports whether the plan changes anything.
func (p Plan) Actionable() bool {
	return len(p.RequireArgs()) > 0
}

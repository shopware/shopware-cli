package directory

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
)

// SchemaVersion is the manifest format version this build understands. It is a
// major-only integer: bump it only on a breaking format change, in the same
// change that teaches the code to read the new format. Additive changes (new
// optional fields, new enum values) are backward compatible and stay on the
// current version.
const SchemaVersion = 1

// namePattern constrains integration names: lowercase alphanumeric with
// hyphens, starting with a letter.
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

var (
	validTypes            = map[Type]bool{TypeSkill: true}
	validKinds            = map[DeliveryKind]bool{DeliveryBundled: true, DeliveryGit: true}
	validStatuses         = map[Status]bool{StatusActive: true, StatusDeprecated: true}
	validCompatibilitySrc = map[string]bool{"owner": true}
)

// Validate checks the directory against the v1 contract (see CONTRACT.md) and
// returns every violation at once, joined via errors.Join. It returns nil when
// the directory is valid.
func (d *Directory) Validate() error {
	var errs []error

	if d.Version != SchemaVersion {
		errs = append(errs, fmt.Errorf("unsupported manifest version %d (this build supports %d)", d.Version, SchemaVersion))
	}
	if len(d.Integrations) == 0 {
		errs = append(errs, errors.New("integrations must not be empty"))
	}

	firstSeen := make(map[string]int) // name -> index of first occurrence
	for i, e := range d.Integrations {
		loc := fmt.Sprintf("integrations[%d]", i)
		if e.Name != "" {
			loc = fmt.Sprintf("integrations[%d] (%s)", i, e.Name)
		}

		switch {
		case e.Name == "":
			errs = append(errs, fmt.Errorf("%s: name is required", loc))
		case !namePattern.MatchString(e.Name):
			errs = append(errs, fmt.Errorf("%s: name %q must match %s", loc, e.Name, namePattern.String()))
		}
		if e.Name != "" {
			if first, dup := firstSeen[e.Name]; dup {
				errs = append(errs, fmt.Errorf("%s: duplicate name %q (first defined at integrations[%d])", loc, e.Name, first))
			} else {
				firstSeen[e.Name] = i
			}
		}

		if !validTypes[e.Type] {
			errs = append(errs, fmt.Errorf("%s: unknown type %q (allowed: skill)", loc, e.Type))
		}
		if e.DisplayName == "" {
			errs = append(errs, fmt.Errorf("%s: display_name is required", loc))
		}
		if e.Provider == "" {
			errs = append(errs, fmt.Errorf("%s: provider is required", loc))
		}
		if e.Description == "" {
			errs = append(errs, fmt.Errorf("%s: description is required", loc))
		}
		if !validStatuses[e.Status] {
			errs = append(errs, fmt.Errorf("%s: unknown status %q (allowed: active, deprecated)", loc, e.Status))
		}

		if e.Documentation == "" {
			errs = append(errs, fmt.Errorf("%s: documentation is required", loc))
		} else if !isAbsoluteHTTPURL(e.Documentation) {
			errs = append(errs, fmt.Errorf("%s: documentation %q must be an absolute http(s) URL", loc, e.Documentation))
		}

		errs = append(errs, validateDelivery(loc, e.Delivery)...)

		if e.Compatibility != nil && !validCompatibilitySrc[e.Compatibility.Source] {
			errs = append(errs, fmt.Errorf("%s: unknown compatibility.source %q (allowed: owner)", loc, e.Compatibility.Source))
		}
		if e.Internal != nil && e.Internal.Maintainer == "" {
			errs = append(errs, fmt.Errorf("%s: internal.maintainer must not be empty when internal is set", loc))
		}
	}

	return errors.Join(errs...)
}

// validateDelivery checks a single entry's delivery block: a known kind, and a
// repository URL that is present exactly when the kind is git.
func validateDelivery(loc string, d Delivery) []error {
	var errs []error

	if !validKinds[d.Kind] {
		errs = append(errs, fmt.Errorf("%s: unknown delivery.kind %q (allowed: bundled, git)", loc, d.Kind))
	}

	if d.Kind == DeliveryGit {
		switch {
		case d.Repository == "":
			errs = append(errs, fmt.Errorf("%s: delivery.repository is required when delivery.kind is %q", loc, DeliveryGit))
		case !isAbsoluteHTTPURL(d.Repository):
			errs = append(errs, fmt.Errorf("%s: delivery.repository %q must be an absolute http(s) URL", loc, d.Repository))
		}
	} else if d.Repository != "" {
		errs = append(errs, fmt.Errorf("%s: delivery.repository is only allowed when delivery.kind is %q", loc, DeliveryGit))
	}

	return errs
}

// isAbsoluteHTTPURL reports whether s is an absolute http(s) URL with a host.
func isAbsoluteHTTPURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

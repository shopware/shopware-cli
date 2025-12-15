package ci

import (
	"context"
	"time"

	"github.com/shopware/shopware-cli/logging"
)

// DefaultCi implements CiHelper for non-CI environments.
type DefaultCi struct{}

type DefaultCiSection struct {
	name  string
	start time.Time
}

// Section starts a new log section.
func (d *DefaultCi) Section(ctx context.Context, name string) Section {
	logging.FromContext(ctx).Infof("Starting %s", name)
	return DefaultCiSection{
		name:  name,
		start: time.Now(),
	}
}

// SectionEnd ends the current log section.
func (d DefaultCiSection) End(ctx context.Context) {
	logging.FromContext(ctx).Infof("%s ended after %s", d.name, time.Since(d.start))
}

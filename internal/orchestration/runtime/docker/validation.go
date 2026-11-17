package docker

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

func validName(s string) bool {
	return s != "" && !strings.HasPrefix(s, "-") && strings.TrimSpace(s) == s && !hasControl(s)
}

func validateMount(m runtime.ContainerMount) error {
	if !filepath.IsAbs(m.Target) || hasControl(m.Target) || strings.Contains(m.Target, ",") {
		return errors.New("invalid mount target")
	}
	switch m.Type {
	case "", runtime.MountBind:
		if !filepath.IsAbs(m.Source) {
			return errors.New("bind mount source must be absolute")
		}
	case runtime.MountVolume:
		if !validName(m.Source) || strings.ContainsAny(m.Source, "/\\") {
			return errors.New("invalid volume name")
		}
	default:
		return fmt.Errorf("%w: mount type %q", runtime.ErrUnsupported, m.Type)
	}
	if hasControl(m.Source) || strings.Contains(m.Source, ",") {
		return errors.New("invalid mount source")
	}
	return nil
}

func mountValue(m runtime.ContainerMount) string {
	kind := string(m.Type)
	if kind == "" {
		kind = "bind"
	}
	s := "type=" + kind + ",src=" + m.Source + ",dst=" + m.Target
	if m.ReadOnly {
		s += ",readonly"
	}
	return s
}

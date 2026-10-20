package deployment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

type PreflightAction string

const (
	PreflightRollout  PreflightAction = "rollout"
	PreflightRollback PreflightAction = "rollback"
)

type PreflightOptions struct {
	Action      PreflightAction
	Environment string
	Deployment  Deployment
}

// Preflight describes the resolved local source and configured remote targets.
// It does not connect to SSH or check remote readiness.
type Preflight struct {
	ProjectDirectory string
	ConfigPath       string
	ArchivePath      string
	Targets          []PreflightTarget
}

type PreflightTarget struct {
	Name           string
	Host           string
	User           string
	Port           int
	Directory      string
	DeploymentRoot string
	Migration      bool
}

type PreflightProvider interface {
	DeploymentPreflight(context.Context, Deployment, PreflightAction) (Preflight, error)
}

// WritePreflight prints a backend's preview before confirmation or activation.
// Backends without this optional capability retain their existing behavior.
func WritePreflight(ctx context.Context, backend Backend, options PreflightOptions, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	provider, ok := backend.(PreflightProvider)
	if !ok {
		return nil
	}
	preview, err := provider.DeploymentPreflight(ctx, options.Deployment, options.Action)
	if err != nil {
		return fmt.Errorf("cannot prepare deployment preflight: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot resolve working directory: %w", err)
	}
	environment := options.Environment
	if environment == "" {
		environment = "local"
	}
	var summary strings.Builder
	summary.WriteString("Deployment preflight\n")
	field := func(label, value string) {
		// Keep paths and configuration values on one line in terminal output.
		if strings.ContainsFunc(value, func(r rune) bool { return !unicode.IsPrint(r) }) {
			value = strconv.Quote(value)
		}
		fmt.Fprintf(&summary, "  %s: %s\n", label, value)
	}
	field("Action", string(options.Action))
	field("Working directory", cwd)
	field("Project directory", preview.ProjectDirectory)
	if preview.ConfigPath != "" {
		field("Project config", preview.ConfigPath)
	}
	field("Environment", environment)
	field("Deployment", options.Deployment.Reference)
	if preview.ArchivePath != "" {
		field("Archive", preview.ArchivePath)
	}
	for _, target := range preview.Targets {
		summary.WriteByte('\n')
		if target.Name != "" {
			field("SSH target", target.Name)
		}
		field("SSH host", target.Host)
		user := target.User
		if user == "" {
			user = "SSH config/default"
		}
		field("SSH user", user)
		port := "SSH config/default"
		// The SSH transport only passes -p for non-default ports.
		if target.Port != 0 && target.Port != 22 {
			port = strconv.Itoa(target.Port)
		}
		field("SSH port", port)
		field("Active path", target.Directory)
		field("Deployment root", target.DeploymentRoot)
		if target.Migration {
			field("Role", "Deployment Helper host")
		}
	}
	summary.WriteByte('\n')
	if _, err := io.WriteString(output, summary.String()); err != nil {
		return fmt.Errorf("cannot display deployment preflight: %w", err)
	}
	return nil
}

func (s *SSH) DeploymentPreflight(ctx context.Context, artifact Deployment, action PreflightAction) (Preflight, error) {
	var preview Preflight
	if err := ctx.Err(); err != nil {
		return preview, err
	}
	if action != PreflightRollout && action != PreflightRollback {
		return preview, errors.New("cannot preview an unsupported deployment action")
	}
	target, err := s.preflightTarget()
	if err != nil {
		return preview, err
	}
	if _, err := sshDeploymentReleaseName(artifact.Reference); err != nil {
		return preview, err
	}
	if action == PreflightRollback && !sshReleaseNamePattern.MatchString(artifact.Reference) {
		return preview, errors.New("cannot roll back without a retained deployment name")
	}
	preview.ProjectDirectory, err = filepath.Abs(s.root)
	if err != nil {
		return preview, fmt.Errorf("cannot resolve project directory: %w", err)
	}
	if s.configPath != "" {
		preview.ConfigPath, err = filepath.Abs(s.configPath)
		if err != nil {
			return preview, fmt.Errorf("cannot resolve project config path: %w", err)
		}
	}
	if action == PreflightRollout {
		archive := resolveDeploymentArchive(s.root, artifact.Reference)
		preview.ArchivePath, err = filepath.Abs(archive)
		if err != nil {
			return preview, fmt.Errorf("cannot resolve deployment archive: %w", err)
		}
		info, err := os.Stat(preview.ArchivePath)
		if err != nil {
			return preview, fmt.Errorf("cannot read deployment archive: %w", err)
		}
		if !info.Mode().IsRegular() {
			return preview, errors.New("cannot use deployment archive: it must be a regular file")
		}
		target.Migration = true
	}
	preview.Targets = []PreflightTarget{target}
	return preview, nil
}

func (s *SSH) preflightTarget() (PreflightTarget, error) {
	var target PreflightTarget
	root, err := s.deploymentRoot()
	if err != nil {
		return target, err
	}
	if s.env == nil || s.env.SSH == nil {
		return target, errors.New("cannot preview SSH deployment without an SSH environment")
	}
	config := s.env.SSH
	if _, _, err := config.SharedPaths(); err != nil {
		return target, err
	}
	if err := config.Cachetool.Validate(); err != nil {
		return target, err
	}
	return PreflightTarget{
		Name: s.name, Host: config.Host, User: config.User, Port: config.Port,
		Directory: path.Join(root, s.activeLinkName()), DeploymentRoot: root,
	}, nil
}

func (g *sshGroup) DeploymentPreflight(ctx context.Context, artifact Deployment, action PreflightAction) (Preflight, error) {
	preview, err := g.hosts[g.migration].backend.DeploymentPreflight(ctx, artifact, action)
	if err != nil {
		return preview, err
	}
	preview.Targets = make([]PreflightTarget, 0, len(g.hosts))
	for i, host := range g.hosts {
		if err := ctx.Err(); err != nil {
			return Preflight{}, err
		}
		target, err := host.backend.preflightTarget()
		if err != nil {
			return Preflight{}, fmt.Errorf("cannot preview SSH target %s: %w", host.name, err)
		}
		target.Name = host.name
		target.Migration = action == PreflightRollout && i == g.migration
		preview.Targets = append(preview.Targets, target)
	}
	return preview, nil
}

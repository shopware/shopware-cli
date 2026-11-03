package ssh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

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

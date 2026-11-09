package ssh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
)

func (s *SSH) DeploymentPreflight(ctx context.Context, artifact Deployment, action PreflightAction) (Preflight, error) {
	var preview Preflight
	if err := ctx.Err(); err != nil {
		return preview, err
	}
	if action != PreflightRollout && action != PreflightRollback {
		return preview, errors.New("cannot preview an unsupported deployment action")
	}
	target, err := s.preflightTarget(s.name)
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
		target.Fields = append(target.Fields, PreflightField{Label: "Role", Value: "Deployment Helper host"})
	}
	preview.Targets = []PreflightTarget{target}
	return preview, nil
}

func (s *SSH) preflightTarget(name string) (PreflightTarget, error) {
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
	if name != "" {
		target.Fields = append(target.Fields, PreflightField{Label: "SSH target", Value: name})
	}
	user := config.User
	if user == "" {
		user = "SSH config/default"
	}
	port := "SSH config/default"
	// The SSH transport only passes -p for non-default ports.
	if config.Port != 0 && config.Port != 22 {
		port = strconv.Itoa(config.Port)
	}
	target.Fields = append(target.Fields,
		PreflightField{Label: "SSH host", Value: config.Host},
		PreflightField{Label: "SSH user", Value: user},
		PreflightField{Label: "SSH port", Value: port},
		PreflightField{Label: "Active path", Value: path.Join(root, s.activeLinkName())},
		PreflightField{Label: "Deployment root", Value: root},
	)
	return target, nil
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
		target, err := host.backend.preflightTarget(host.name)
		if err != nil {
			return Preflight{}, fmt.Errorf("cannot preview SSH target %s: %w", host.name, err)
		}
		if action == PreflightRollout && i == g.migration {
			target.Fields = append(target.Fields, PreflightField{Label: "Role", Value: "Deployment Helper host"})
		}
		preview.Targets = append(preview.Targets, target)
	}
	return preview, nil
}

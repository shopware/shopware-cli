package deployment

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

func (s *SSH) RollbackCandidates(ctx context.Context) ([]Candidate, error) {
	raw, err := s.rollbackCandidates(ctx)
	candidates := make([]Candidate, len(raw))
	for i := range raw {
		candidates[i] = raw[i].Candidate
	}
	return candidates, err
}

type sshCandidate struct {
	Candidate
	SHA256 string `json:"sha256"`
}

func (s *SSH) rollbackCandidates(ctx context.Context) ([]sshCandidate, error) {
	root, err := s.deploymentRoot()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(sshRolloutInput{Root: root, Action: "candidates"})
	if err != nil {
		return nil, err
	}
	cmd := s.transport.RemotePHPCommand(ctx, "-r", strings.TrimPrefix(sshDeploymentScript, "<?php\n"), "--", string(payload)).Cmd
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("discover SSH rollback candidates: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	var candidates []sshCandidate
	if err := json.Unmarshal(output, &candidates); err != nil {
		return nil, fmt.Errorf("decode SSH rollback candidates: %w", err)
	}
	return candidates, nil
}

func (s *SSH) ActivateDeployment(ctx context.Context, deployment Deployment, output io.Writer) (Rollout, error) {
	root, err := s.deploymentRoot()
	if err != nil {
		return Rollout{}, err
	}
	if !sshReleaseNamePattern.MatchString(deployment.Reference) {
		return Rollout{}, fmt.Errorf("invalid retained deployment name %q", deployment.Reference)
	}
	cachetool, err := s.cachetoolInput()
	if err != nil {
		return Rollout{}, err
	}
	input := sshRolloutInput{
		Action: "activate", Root: root,
		Reference: time.Now().UTC().Format("20060102T150405Z") + "-" + rand.Text(),
		Release:   deployment.Reference, Deployment: deployment.Reference,
		Cachetool:    cachetool,
		ProbePHPHost: cachetool == nil,
	}
	if s.env != nil && s.env.SSH != nil {
		input.PHP = s.env.SSH.PHPBinary
	}
	return s.runSSHDeployment(ctx, nil, input, output)
}

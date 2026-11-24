package ssh

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
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
	output, err := s.deploymentCommandOutput(ctx, "deploy", sshRolloutInput{Root: root, Current: s.activeLinkName(), Action: sshDeploymentCandidates})
	if err != nil {
		return nil, fmt.Errorf("discover SSH rollback candidates: %w", err)
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
		Action: sshDeploymentActivate, Root: root, Current: s.activeLinkName(),
		Reference: time.Now().UTC().Format("20060102T150405Z") + "-" + rand.Text(),
		Release:   deployment.Reference, Deployment: deployment.Reference,
		Cachetool:    cachetool,
		ProbePHPHost: cachetool == nil,
	}
	if s.env != nil && s.env.SSH != nil {
		input.PHP = s.env.SSH.PHPBinary
		input.PHPArgs = s.env.SSH.PHPArgs
	}
	return s.runSSHDeployment(ctx, nil, input, output)
}

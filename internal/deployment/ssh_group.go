package deployment

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/shopware/shopware-cli/internal/archiver"
	"github.com/shopware/shopware-cli/internal/shop"
)

type sshHost struct {
	name    string
	backend *SSH
}

type sshGroup struct {
	hosts       []sshHost
	migration   int
	parallelism int
}

func newSSHGroup(hosts []sshHost, migrationHost string, parallelism int) (*sshGroup, error) {
	if len(hosts) == 0 || parallelism < 0 {
		return nil, errors.New("SSH group requires hosts and nonnegative parallelism")
	}
	if parallelism == 0 {
		parallelism = 2
	}
	g := &sshGroup{hosts: slices.Clone(hosts), migration: -1, parallelism: parallelism}
	slices.SortFunc(g.hosts, func(a, b sshHost) int { return strings.Compare(a.name, b.name) })
	for i, host := range g.hosts {
		if host.name == "" || host.backend == nil || (i > 0 && g.hosts[i-1].name == host.name) {
			return nil, errors.New("SSH group requires unique, nonempty host names and backends")
		}
		if host.name == migrationHost {
			g.migration = i
		}
	}
	if g.migration < 0 {
		return nil, fmt.Errorf("SSH migration host %q is not a group member", migrationHost)
	}
	return g, nil
}

func (g *sshGroup) Type() string { return "ssh" }

func (g *sshGroup) CreateDeployment(ctx context.Context, options CreateOptions) (Deployment, error) {
	return g.hosts[g.migration].backend.CreateDeployment(ctx, options)
}

func (g *sshGroup) RolloutCandidates(ctx context.Context) ([]Candidate, error) {
	return g.hosts[g.migration].backend.RolloutCandidates(ctx)
}

func (g *sshGroup) WriteDeploymentLogs(ctx context.Context, deployment Deployment, output io.Writer) error {
	return g.hosts[g.migration].backend.WriteDeploymentLogs(ctx, deployment, output)
}

// parallel waits for every worker before returning; all opened sessions remain
// owned by the coordinator even when a different worker fails.
func (g *sshGroup) parallel(ctx context.Context, run func(int) error) error {
	var wg sync.WaitGroup
	limit := make(chan struct{}, g.parallelism)
	errs := make([]error, len(g.hosts))
	for i := range g.hosts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case limit <- struct{}{}:
				defer func() { <-limit }()
			case <-ctx.Done():
				errs[i] = fmt.Errorf("host %s: operation cancelled; state unknown: %w", g.hosts[i].name, ctx.Err())
				return
			}
			if err := run(i); err != nil {
				errs[i] = fmt.Errorf("host %s: %w", g.hosts[i].name, err)
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

func newestTime(a, b *time.Time) int {
	if a == nil {
		if b == nil {
			return 0
		}
		return 1
	}
	if b == nil {
		return -1
	}
	return b.Compare(*a)
}

func (g *sshGroup) ListRollouts(ctx context.Context) ([]Rollout, error) {
	rows := make([][]Rollout, len(g.hosts))
	err := g.parallel(ctx, func(i int) error {
		var err error
		rows[i], err = g.hosts[i].backend.ListRollouts(ctx)
		if err != nil {
			return fmt.Errorf("unavailable; deployment state unknown: %w", err)
		}
		return nil
	})
	var result []Rollout
	for i, hostRows := range rows {
		for _, row := range hostRows {
			row.Host = g.hosts[i].name
			result = append(result, row)
		}
	}
	slices.SortFunc(result, func(a, b Rollout) int {
		if order := newestTime(a.DeployedAt, b.DeployedAt); order != 0 {
			return order
		}
		if order := strings.Compare(a.Host, b.Host); order != 0 {
			return order
		}
		return strings.Compare(a.Reference, b.Reference)
	})
	return result, err
}

func (g *sshGroup) rollbackCandidates(ctx context.Context) ([]sshCandidate, error) {
	rows := make([][]sshCandidate, len(g.hosts))
	if err := g.parallel(ctx, func(i int) error {
		var err error
		rows[i], err = g.hosts[i].backend.rollbackCandidates(ctx)
		return err
	}); err != nil {
		return nil, err
	}
	var result []sshCandidate
	for _, candidate := range rows[g.migration] {
		present := true
		for _, hostRows := range rows {
			index := slices.IndexFunc(hostRows, func(other sshCandidate) bool {
				return other.Deployment.Reference == candidate.Deployment.Reference &&
					other.SHA256 != "" && other.SHA256 == candidate.SHA256
			})
			if index < 0 {
				present = false
				break
			}
			candidate.Active = candidate.Active && hostRows[index].Active
		}
		if present {
			result = append(result, candidate)
		}
	}
	// The migration host supplies stable release creation dates.
	slices.SortFunc(result, func(a, b sshCandidate) int {
		if order := newestTime(a.CreatedAt, b.CreatedAt); order != 0 {
			return order
		}
		return strings.Compare(a.Deployment.Reference, b.Deployment.Reference)
	})
	return result, nil
}

func (g *sshGroup) RollbackCandidates(ctx context.Context) ([]Candidate, error) {
	rows, err := g.rollbackCandidates(ctx)
	result := make([]Candidate, len(rows))
	for i := range rows {
		result[i] = rows[i].Candidate
	}
	return result, err
}

func (g *sshGroup) ActivateDeployment(ctx context.Context, deployment Deployment, output io.Writer) (Rollout, error) {
	if !sshReleaseNamePattern.MatchString(deployment.Reference) {
		return Rollout{}, fmt.Errorf("invalid retained deployment name %q", deployment.Reference)
	}
	rows, err := g.rollbackCandidates(ctx)
	if err != nil {
		return Rollout{}, err
	}
	for _, row := range rows {
		if row.Deployment.Reference == deployment.Reference {
			return g.coordinate(ctx, deployment, sshRolloutInput{
				Action: "activate", Release: deployment.Reference, SHA256: row.SHA256,
			}, nil, output)
		}
	}
	return Rollout{}, errors.New("deployment is not retained with the same checksum on every SSH host")
}

func (g *sshGroup) RolloutDeployment(ctx context.Context, deployment Deployment, output io.Writer) (result Rollout, err error) {
	release, err := sshDeploymentReleaseName(deployment.Reference)
	if err != nil {
		return result, err
	}
	archive := resolveDeploymentArchive(g.hosts[g.migration].backend.root, deployment.Reference)
	source, err := os.Open(archive)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, source.Close()) }()
	info, err := source.Stat()
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() {
		return result, errors.New("deployment archive must be a regular file")
	}
	// Snapshot once: concurrent uploads must use exactly the validated bytes,
	// even if the original local archive is replaced or modified during rollout.
	snapshot, err := os.CreateTemp("", "shopware-cohort-*.tar.gz")
	if err != nil {
		return result, err
	}
	defer func() {
		err = errors.Join(err, snapshot.Close(), os.Remove(snapshot.Name()))
	}()
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(snapshot, hash), archiver.ContextReader(ctx, source)); err != nil {
		return result, fmt.Errorf("read deployment archive: %w", err)
	}
	size, err := snapshot.Seek(0, io.SeekCurrent)
	if err != nil {
		return result, err
	}
	return g.coordinate(ctx, deployment, sshRolloutInput{
		Release: release, Archive: archive, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: size,
	}, snapshot, output)
}

type sshDeploymentSession struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	reader    *bufio.Reader
	input     sshRolloutInput
	result    Rollout
	unchanged bool
	output    io.Writer
}

// hostWriter is private to one stream; the shared downstream writer serializes
// complete prefixed lines across all remote processes.
type hostWriter struct {
	output  io.Writer
	name    string
	pending []byte
}

func (w *hostWriter) Write(data []byte) (int, error) {
	for _, b := range data {
		w.pending = append(w.pending, b)
		if b == '\n' || len(w.pending) >= 8192 {
			if err := w.flush(); err != nil {
				return 0, err
			}
		}
	}
	return len(data), nil
}

func (w *hostWriter) flush() error {
	if len(w.pending) == 0 {
		return nil
	}
	line := "[" + w.name + "] " + strings.TrimSuffix(string(w.pending), "\n") + "\n"
	w.pending = w.pending[:0]
	_, err := io.WriteString(w.output, line)
	return err
}

func (s *SSH) openDeploymentSession(ctx context.Context, input sshRolloutInput, output io.Writer) (*sshDeploymentSession, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	cmd := s.transport.RemotePHPCommand(ctx, "-r", strings.TrimPrefix(sshDeploymentScript, "<?php\n"), "--", string(payload)).Cmd
	cmd.Stderr = output
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	return &sshDeploymentSession{cmd: cmd, stdin: stdin, stdout: stdout, reader: bufio.NewReader(stdout), input: input, output: output}, nil
}

func (s *sshDeploymentSession) close() error {
	// EOF never authorizes preparation or activation. Completed nodes wait for
	// this EOF too, keeping their lock until every activation has been attempted.
	_ = s.stdin.Close()
	err := s.cmd.Wait()
	_ = s.stdout.Close()
	if output, ok := s.output.(*hostWriter); ok {
		err = errors.Join(err, output.flush())
	}
	return err
}

func (s *sshDeploymentSession) stage(ctx context.Context, archive io.Reader) error {
	action, err := readRolloutAction(s.reader, s.input.Reference)
	if err != nil {
		return err
	}
	if action == "UNCHANGED" {
		reference, err := s.reader.ReadString('\n')
		if err != nil {
			return err
		}
		if !sshReleaseNamePattern.MatchString(strings.TrimSuffix(reference, "\n")) {
			return errors.New("invalid unchanged rollout reference")
		}
		if err := json.NewDecoder(s.reader).Decode(&s.result); err != nil {
			return err
		}
		if !s.result.Active || !s.result.Unchanged || s.result.Reference != strings.TrimSuffix(reference, "\n") {
			return errors.New("invalid unchanged rollout result")
		}
		s.unchanged = true
		return nil
	}
	if s.input.Action == "activate" && action != "REUSE" {
		return fmt.Errorf("unexpected retained activation action %s", action)
	}
	if action != "REUSE" {
		if action == "UPLOAD" {
			if archive == nil {
				return errors.New("upload requested without archive")
			}
			if _, err := io.CopyN(s.stdin, archiver.ContextReader(ctx, archive), s.input.Size); err != nil {
				return err
			}
		}
		if err := readRolloutMessage(s.reader, "PREPARE "+s.input.Reference); err != nil {
			return err
		}
		if err := s.authorize(ctx, "CONTINUE"); err != nil {
			return err
		}
	}
	return readRolloutMessage(s.reader, "STAGED "+s.input.Reference)
}

func (s *sshDeploymentSession) authorize(ctx context.Context, action string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := fmt.Fprintln(s.stdin, action+" "+s.input.Reference)
	return err
}

func (s *sshDeploymentSession) prepare(ctx context.Context) error {
	if s.unchanged {
		return nil
	}
	if err := s.authorize(ctx, "PREPARE"); err != nil {
		return err
	}
	return readRolloutMessage(s.reader, "READY "+s.input.Reference)
}

func (s *sshDeploymentSession) activate(ctx context.Context) error {
	if s.unchanged {
		return nil
	}
	if err := s.authorize(ctx, "ACTIVATE"); err != nil {
		return err
	}
	if err := readRolloutMessage(s.reader, "ACTIVE "+s.input.Reference); err != nil {
		return err
	}
	if err := json.NewDecoder(s.reader).Decode(&s.result); err != nil {
		return err
	}
	if !s.result.Active || s.result.Unchanged || s.result.Reference != s.input.Reference {
		return errors.New("invalid activation result")
	}
	return nil
}

func (g *sshGroup) coordinate(ctx context.Context, deployment Deployment, base sshRolloutInput, archive *os.File, output io.Writer) (result Rollout, err error) {
	if output == nil {
		output = io.Discard
	}
	output = &synchronizedWriter{output: output}
	base.Coordinated = true
	base.Reference = time.Now().UTC().Format("20060102T150405Z") + "-" + rand.Text()
	base.Deployment = deployment.Reference
	sessions := make([]*sshDeploymentSession, len(g.hosts))
	statuses := make([]string, len(g.hosts))
	for i := range statuses {
		statuses[i] = "not activated"
	}
	defer func() {
		for i, session := range sessions {
			if session != nil {
				if closeErr := session.close(); closeErr != nil {
					err = errors.Join(err, fmt.Errorf("host %s session: %w", g.hosts[i].name, closeErr))
				}
			}
		}
		if err != nil {
			var summary []string
			for i, status := range statuses {
				summary = append(summary, g.hosts[i].name+"="+status)
			}
			err = fmt.Errorf("SSH cohort %s incomplete (%s); inspect host state before retrying; no automatic rollback: %w", base.Reference, strings.Join(summary, ", "), err)
			result = Rollout{}
		}
	}()
	err = g.parallel(ctx, func(i int) error {
		host := g.hosts[i]
		input := base
		input.Follower = i != g.migration
		var err error
		if input.Root, err = host.backend.deploymentRoot(); err != nil {
			return err
		}
		if input.Cachetool, err = host.backend.cachetoolInput(); err != nil {
			return err
		}
		input.ProbePHPHost = input.Cachetool == nil
		var config *shop.EnvironmentSSHConfig
		if host.backend.env != nil {
			config = host.backend.env.SSH
		}
		if config != nil {
			input.PHP = config.PHPBinary
		}
		if input.SharedFiles, input.SharedDirectories, err = config.SharedPaths(); err != nil {
			return err
		}
		session, err := host.backend.openDeploymentSession(ctx, input, &hostWriter{output: output, name: host.name})
		if err != nil {
			statuses[i] = "failed staging; state unknown"
			return err
		}
		sessions[i] = session
		var reader io.Reader
		if archive != nil {
			reader = io.NewSectionReader(archive, 0, input.Size)
		}
		if err := session.stage(ctx, reader); err != nil {
			statuses[i] = "failed staging; not activated"
			return err
		}
		if session.unchanged {
			statuses[i] = "unchanged"
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	// Global work is authorized only after every member is safely staged.
	if err = sessions[g.migration].prepare(ctx); err != nil {
		statuses[g.migration] = "failed preparation; not activated"
		return result, fmt.Errorf("migration host %s: %w", g.hosts[g.migration].name, err)
	}
	if err = g.parallel(ctx, func(i int) error {
		if i == g.migration {
			return nil
		}
		if err := sessions[i].prepare(ctx); err != nil {
			statuses[i] = "failed preparation; not activated"
			return err
		}
		return nil
	}); err != nil {
		return result, err
	}
	changed := false
	for i, session := range sessions {
		if session.unchanged {
			continue
		}
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if err = session.activate(ctx); err != nil {
			statuses[i] = "failed activation; state unknown"
			return result, fmt.Errorf("host %s activation: %w", g.hosts[i].name, err)
		}
		statuses[i] = "activated"
		changed = true
	}
	if !changed {
		return sessions[g.migration].result, nil
	}
	now := time.Now().UTC()
	return Rollout{Reference: base.Reference, Deployment: deployment, Active: true, DeployedAt: &now}, nil
}

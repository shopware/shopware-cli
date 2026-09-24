package deployment

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/shopware/shopware-cli/internal/archiver"
	"github.com/shopware/shopware-cli/internal/ci"
	"github.com/shopware/shopware-cli/internal/shop"
)

//go:embed ssh_deployment.php
var sshDeploymentScript string

//go:embed ssh_deployment_list.php
var sshDeploymentListScript string

var sshReleaseNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

type synchronizedWriter struct {
	mutex  sync.Mutex
	output io.Writer
}

func (w *synchronizedWriter) Write(data []byte) (int, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.output.Write(data)
}

func (w *synchronizedWriter) UnwrapWriter() io.Writer {
	return w.output
}

type sshRolloutInput struct {
	Root              string   `json:"root"`
	Reference         string   `json:"reference"`
	Release           string   `json:"release"`
	Deployment        string   `json:"deployment"`
	Archive           string   `json:"archive"`
	SHA256            string   `json:"sha256"`
	Size              int64    `json:"size"`
	PHP               string   `json:"php,omitempty"`
	SharedFiles       []string `json:"shared_files"`
	SharedDirectories []string `json:"shared_directories"`
}

// RolloutDeployment prepares an isolated release, then atomically switches
// current. No existing release is deleted, and no database rollback is implied.
func (s *SSH) RolloutDeployment(ctx context.Context, deployment Deployment, output io.Writer) (result Rollout, err error) {
	archive := resolveDeploymentArchive(s.root, deployment.Reference)
	return s.rolloutArchive(ctx, deployment, archive, output)
}

// rolloutArchive retains the deployment identity independently of its local path.
func (s *SSH) rolloutArchive(ctx context.Context, deployment Deployment, archive string, output io.Writer) (result Rollout, err error) {
	root, err := s.deploymentRoot()
	if err != nil {
		return result, err
	}
	var sshConfig *shop.EnvironmentSSHConfig
	if s.env != nil {
		sshConfig = s.env.SSH
	}
	sharedFiles, sharedDirectories, err := sshConfig.SharedPaths()
	if err != nil {
		return result, err
	}
	if deployment.Reference == "" {
		return result, errors.New("deployment archive reference must not be empty")
	}
	if archive == "" {
		return result, errors.New("deployment archive path must not be empty")
	}
	release, err := sshDeploymentReleaseName(deployment.Reference)
	if err != nil {
		return result, err
	}
	archivePath, err := filepath.Abs(archive)
	if err != nil {
		return result, err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return result, fmt.Errorf("open deployment archive: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() {
		return result, errors.New("deployment archive must be a regular file")
	}
	hash := sha256.New()
	if err := archiver.ValidateTarGz(ctx, io.TeeReader(file, hash)); err != nil {
		return result, fmt.Errorf("validate deployment archive: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	input := sshRolloutInput{
		Root: root, Reference: time.Now().UTC().Format("20060102T150405Z") + "-" + rand.Text(),
		Release: release, Deployment: deployment.Reference, Archive: archivePath,
		SHA256: hex.EncodeToString(hash.Sum(nil)), Size: info.Size(),
		SharedFiles: sharedFiles, SharedDirectories: sharedDirectories,
	}
	if sshConfig != nil {
		// Preserve configured wrappers and their PHP flags when invoking the helper.
		input.PHP = sshConfig.PHPBinary
	}
	return s.runSSHDeployment(ctx, file, input, output)
}

func sshDeploymentReleaseName(reference string) (string, error) {
	name := strings.TrimSuffix(path.Base(strings.ReplaceAll(reference, "\\", "/")), ".tar.gz")
	if !sshReleaseNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid deployment name %q: use 1-128 ASCII letters, digits, dots, hyphens or underscores, starting with a letter or digit", name)
	}
	return name, nil
}

func (s *SSH) deploymentRoot() (string, error) {
	current := path.Clean(s.directory)
	if !path.IsAbs(current) || path.Base(current) != "current" || path.Dir(current) == "/" {
		return "", errors.New("SSH deployment requires ssh.directory to be an absolute path ending in /current (for example /var/www/shop/current)")
	}
	return path.Dir(current), nil
}

// ListRollouts returns successful retained releases in newest-first order.
func (s *SSH) ListRollouts(ctx context.Context) ([]Rollout, error) {
	root, err := s.deploymentRoot()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(struct {
		Root string `json:"root"`
	}{Root: root})
	if err != nil {
		return nil, err
	}
	cmd := s.transport.RemotePHPCommand(ctx, "-r", strings.TrimPrefix(sshDeploymentListScript, "<?php\n"), "--", string(payload)).Cmd
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return nil, fmt.Errorf("list SSH rollouts: %s: %w", message, err)
		}
		return nil, fmt.Errorf("list SSH rollouts: %w", err)
	}
	var rollouts []Rollout
	if err := json.Unmarshal(output, &rollouts); err != nil {
		return nil, fmt.Errorf("decode SSH rollouts: %w", err)
	}
	for i := range rollouts {
		reference := rollouts[i].Deployment.Reference
		if strings.HasSuffix(reference, ".tar.gz") {
			// Metadata may contain archive paths from a different checkout or OS.
			rollouts[i].Deployment.Name = strings.TrimSuffix(path.Base(strings.ReplaceAll(reference, "\\", "/")), ".tar.gz")
		}
	}
	return rollouts, nil
}

func (s *SSH) runSSHDeployment(ctx context.Context, archive io.Reader, input sshRolloutInput, output io.Writer) (Rollout, error) {
	if output == nil {
		output = io.Discard
	}
	output = &synchronizedWriter{output: output}
	payload, err := json.Marshal(input)
	if err != nil {
		return Rollout{}, err
	}
	// Do not cd into current: it need not exist for the first deployment.
	cmd := s.transport.RemotePHPCommand(ctx, "-r", strings.TrimPrefix(sshDeploymentScript, "<?php\n"), "--", string(payload)).Cmd
	cmd.Stderr = output
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Rollout{}, err
	}
	defer func() { _ = stdin.Close() }()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Rollout{}, err
	}
	defer func() { _ = stdout.Close() }()
	if err := cmd.Start(); err != nil {
		return Rollout{}, err
	}
	result, protocolErr := exchangeSSHDeployment(ctx, stdin, bufio.NewReader(stdout), archive, input, ci.New(output))
	// EOF also tells the remote side to abandon a prepared release if the
	// client could not authorize activation. The remote flock is process-owned.
	closeErr := stdin.Close()
	waitErr := cmd.Wait()
	if err := errors.Join(ctx.Err(), protocolErr, closeErr, waitErr); err != nil {
		return Rollout{}, fmt.Errorf("SSH rollout %s failed; if activation was interrupted, inspect %s/current before retrying: %w", input.Reference, input.Root, err)
	}
	return result, nil
}

func exchangeSSHDeployment(ctx context.Context, stdin io.Writer, stdout *bufio.Reader, archive io.Reader, input sshRolloutInput, sections ci.Helper) (Rollout, error) {
	action, err := readRolloutAction(stdout, input.Reference)
	if err != nil {
		return Rollout{}, fmt.Errorf("negotiate deployment: %w", err)
	}
	if action == "UNCHANGED" {
		line, err := stdout.ReadSlice('\n')
		if err != nil {
			return Rollout{}, err
		}
		reference := strings.TrimSuffix(string(line), "\n")
		if !sshReleaseNamePattern.MatchString(reference) {
			return Rollout{}, fmt.Errorf("invalid existing rollout reference %q", reference)
		}
		return Rollout{Reference: reference, Deployment: Deployment{Reference: input.Deployment}, Active: true, Unchanged: true}, nil
	}
	if action == "REUSE" {
		section := sections.Section("Reusing prepared release")
		err = readRolloutMessage(stdout, "READY "+input.Reference)
		section.End()
	} else {
		err = prepareSSHRelease(ctx, stdin, stdout, archive, input, sections, action == "UPLOAD")
	}
	if err != nil {
		return Rollout{}, err
	}
	if err := ctx.Err(); err != nil {
		return Rollout{}, err
	}
	activationSection := sections.Section("Activating release")
	defer activationSection.End()
	if _, err := fmt.Fprintln(stdin, "ACTIVATE "+input.Reference); err != nil {
		return Rollout{}, err
	}
	if err := readRolloutMessage(stdout, "ACTIVE "+input.Reference); err != nil {
		return Rollout{}, err
	}
	return Rollout{Reference: input.Reference, Deployment: Deployment{Reference: input.Deployment}, Active: true}, nil
}

func prepareSSHRelease(ctx context.Context, stdin io.Writer, stdout *bufio.Reader, archive io.Reader, input sshRolloutInput, sections ci.Helper, upload bool) error {
	artifactSectionName := "Using cached deployment artifact"
	if upload {
		artifactSectionName = "Uploading deployment artifact"
	}
	artifactSection := sections.Section(artifactSectionName)
	var err error
	if upload {
		_, err = io.CopyN(stdin, archiver.ContextReader(ctx, archive), input.Size)
	}
	artifactSection.End()
	if err != nil {
		return fmt.Errorf("upload deployment archive: %w", err)
	}

	if err := readRolloutMessage(stdout, "PREPARE "+input.Reference); err != nil {
		return fmt.Errorf("start release preparation: %w", err)
	}
	prepareSection := sections.Section("Preparing release")
	if _, err := fmt.Fprintln(stdin, "CONTINUE "+input.Reference); err != nil {
		prepareSection.End()
		return err
	}
	err = readRolloutMessage(stdout, "READY "+input.Reference)
	prepareSection.End()
	if err != nil {
		return fmt.Errorf("prepare release: %w", err)
	}
	return nil
}

func readRolloutAction(reader *bufio.Reader, reference string) (string, error) {
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return "", err
	}
	action, id, ok := strings.Cut(strings.TrimSuffix(string(line), "\n"), " ")
	if ok && id == reference {
		switch action {
		case "UPLOAD", "CACHED", "REUSE", "UNCHANGED":
			return action, nil
		}
	}
	return "", fmt.Errorf("unexpected SSH deployment response %q", line)
}

func readRolloutMessage(reader *bufio.Reader, expected string) error {
	// A bounded protocol message also prevents unexpected remote stdout from
	// consuming unbounded memory. Build/helper output belongs on stderr.
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return err
	}
	if strings.TrimSuffix(string(line), "\n") != expected {
		return fmt.Errorf("unexpected SSH deployment response %q", line)
	}
	return nil
}

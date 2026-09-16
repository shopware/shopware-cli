package executor

import (
	"bufio"
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
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/archiver"
	"github.com/shopware/shopware-cli/internal/shop"
)

//go:embed ssh_deployment.php
var sshDeploymentScript string

type sshRolloutInput struct {
	Root              string   `json:"root"`
	Reference         string   `json:"reference"`
	Deployment        string   `json:"deployment"`
	SHA256            string   `json:"sha256"`
	Size              int64    `json:"size"`
	PHP               string   `json:"php"`
	SharedFiles       []string `json:"shared_files"`
	SharedDirectories []string `json:"shared_directories"`
}

// RolloutDeployment prepares an isolated release, then atomically switches
// current. No existing release is deleted, and no database rollback is implied.
func (s *SSHExecutor) RolloutDeployment(ctx context.Context, deployment Deployment) (result Rollout, err error) {
	current := path.Clean(s.directory)
	if !path.IsAbs(current) || path.Base(current) != "current" || path.Dir(current) == "/" {
		return result, errors.New("SSH deployment requires ssh.directory to be an absolute path ending in /current (for example /var/www/shop/current)")
	}
	var sshConfig *shop.EnvironmentSSHConfig
	if s.envCfg != nil {
		sshConfig = s.envCfg.SSH
	}
	sharedFiles, sharedDirectories, err := sshConfig.SharedPaths()
	if err != nil {
		return result, err
	}
	if deployment.Reference == "" {
		return result, errors.New("deployment archive reference must not be empty")
	}
	archivePath, err := filepath.Abs(deployment.Reference)
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
		Root: path.Dir(current), Reference: time.Now().UTC().Format("20060102T150405Z") + "-" + rand.Text(),
		Deployment: archivePath, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: info.Size(), PHP: s.php(),
		SharedFiles: sharedFiles, SharedDirectories: sharedDirectories,
	}
	if err := s.runSSHDeployment(ctx, file, input); err != nil {
		return result, err
	}
	return Rollout{Reference: input.Reference, Deployment: deployment, Active: true}, nil
}

func (s *SSHExecutor) runSSHDeployment(ctx context.Context, archive io.Reader, input sshRolloutInput) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	// Do not cd into current: it need not exist for the first deployment.
	remote := "exec " + shellQuoteArg(s.php()) + " -r " + shellQuoteArg(strings.TrimPrefix(sshDeploymentScript, "<?php\n")) + " -- " + shellQuoteArg(string(payload))
	cmd := exec.CommandContext(ctx, "ssh", append(s.sshArgs(), "-T", s.target(), remote)...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer func() { _ = stdin.Close() }()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	defer func() { _ = stdout.Close() }()
	if err := cmd.Start(); err != nil {
		return err
	}
	protocolErr := exchangeSSHDeployment(ctx, stdin, bufio.NewReader(stdout), archive, input)
	// EOF also tells the remote side to abandon a prepared release if the
	// client could not authorize activation. The remote flock is process-owned.
	closeErr := stdin.Close()
	waitErr := cmd.Wait()
	if err := errors.Join(ctx.Err(), protocolErr, closeErr, waitErr); err != nil {
		return fmt.Errorf("SSH rollout %s failed; if activation was interrupted, inspect %s/current before retrying: %w", input.Reference, input.Root, err)
	}
	return nil
}

func exchangeSSHDeployment(ctx context.Context, stdin io.Writer, stdout *bufio.Reader, archive io.Reader, input sshRolloutInput) error {
	if _, err := io.CopyN(stdin, archiver.ContextReader(ctx, archive), input.Size); err != nil {
		return fmt.Errorf("upload deployment archive: %w", err)
	}
	if err := readRolloutMessage(stdout, "READY "+input.Reference); err != nil {
		return fmt.Errorf("prepare release: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdin, "ACTIVATE "+input.Reference); err != nil {
		return err
	}
	return readRolloutMessage(stdout, "ACTIVE "+input.Reference)
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

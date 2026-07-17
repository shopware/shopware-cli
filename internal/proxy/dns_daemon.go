//go:build !windows

package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	dnsPIDFileName = "dns.pid"
	dnsLogFileName = "dns.log"
	// dnsDomainFileName records which domain the running daemon answers,
	// so a daemon serving an outdated domain is restarted.
	dnsDomainFileName = "dns.domain"
)

// EnsureDNSServerRunning starts the embedded DNS server as a detached
// background process if it is not already running for baseDomain. A daemon
// answering a different domain is restarted. It is idempotent.
func EnsureDNSServerRunning(baseDomain string) error {
	if running, _, _ := DNSServerStatus(); running {
		if runningDNSDomain() == baseDomain {
			return nil
		}

		if err := StopDNSServer(); err != nil {
			return err
		}
	}

	return spawnDNSServer(baseDomain)
}

// runningDNSDomain returns the domain the current daemon was started for.
func runningDNSDomain() string {
	dir, err := StateDir()
	if err != nil {
		return ""
	}

	data, err := os.ReadFile(filepath.Join(dir, dnsDomainFileName))
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

// DNSServerStatus reports whether the DNS daemon is alive, based on its PID
// file. A stale PID file (process gone, e.g. after a reboot) counts as not
// running.
func DNSServerStatus() (bool, int, error) {
	dir, err := StateDir()
	if err != nil {
		return false, 0, err
	}

	data, err := os.ReadFile(filepath.Join(dir, dnsPIDFileName))
	if os.IsNotExist(err) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false, 0, nil //nolint:nilerr // an unreadable PID file just means: not running
	}

	// Signal 0 only checks whether the process exists.
	if err := syscall.Kill(pid, 0); err != nil {
		return false, 0, nil //nolint:nilerr // a dead PID (e.g. after reboot) just means: not running
	}

	return true, pid, nil
}

// StopDNSServer terminates the DNS daemon if it is running and removes its
// PID file.
func StopDNSServer() error {
	running, pid, err := DNSServerStatus()
	if err != nil {
		return err
	}

	if running {
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			return fmt.Errorf("stopping DNS server (pid %d): %w", pid, err)
		}
	}

	dir, err := StateDir()
	if err != nil {
		return err
	}

	err = os.Remove(filepath.Join(dir, dnsPIDFileName))
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// spawnDNSServer re-executes the current binary with the hidden
// internal-dns-serve subcommand as a detached background process, logging to
// dns.log in the state directory.
func spawnDNSServer(baseDomain string) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	dir, err := StateDir()
	if err != nil {
		return err
	}

	logFile, err := os.OpenFile(filepath.Join(dir, dnsLogFileName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = logFile.Close() }()

	//nolint:noctx // the daemon must outlive this process, so it is deliberately not bound to a context
	cmd := exec.Command(exePath, "project", "proxy", "internal-dns-serve", "--port", strconv.Itoa(DNSPort), "--domain", baseDomain)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Detach from the current session so the daemon outlives this process
	// and is not bound to the terminal.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return err
	}

	pid := cmd.Process.Pid
	// The child is deliberately not waited on; releasing avoids a zombie in
	// the short window before this process exits.
	_ = cmd.Process.Release()

	if err := os.WriteFile(filepath.Join(dir, dnsDomainFileName), []byte(baseDomain), 0o600); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, dnsPIDFileName), []byte(strconv.Itoa(pid)), 0o600)
}

package executor

import (
	"bufio"
	"context"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/shop"
)

// SSHProbe reports what a single non-interactive SSH connection found on the
// target of an ssh environment.
type SSHProbe struct {
	// Connected is true once a command ran on the remote host.
	Connected bool
	Duration  time.Duration
	// Error holds the ssh client's message when the connection failed.
	Error string
	// DeploymentPHP is true when PHP runs through sudo as ssh.become.
	// It is checked even before the first rollout.
	DeploymentPHP bool

	// ParentDirectory is true when the parent of ssh.directory exists. A
	// deployment target has only that before its first rollout.
	ParentDirectory bool
	// Directory is true when ssh.directory exists on the remote host. The
	// remaining fields are only checked inside that directory.
	Directory        bool
	PHP              bool
	Console          bool
	Logs             bool
	DeploymentHelper bool
}

// ProbeSSH connects to the resolved single-host SSH configuration and inspects
// the project directory. It never prompts: password and passphrase prompts
// would block callers that show progress, so authentication has to work
// through keys or the ssh agent.
func ProbeSSH(ctx context.Context, cfg *shop.EnvironmentSSHConfig) SSHProbe {
	s := &SSHExecutor{host: cfg.Host, user: cfg.User, port: cfg.Port, directory: cfg.Directory, identityFile: cfg.IdentityFile, phpBinary: cfg.PHPBinary, phpArgs: cfg.PHPArgs}

	args := append(s.sshArgs(), "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-T", "--", s.target(), s.probeScript(cfg.Become))
	cmd := exec.CommandContext(ctx, "ssh", args...)
	logCmd(ctx, cmd)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()

	probe := parseSSHProbe(stdout.String())
	probe.Duration = time.Since(start)
	if !probe.Connected {
		probe.Error = strings.TrimSpace(stderr.String())
		if probe.Error == "" && err != nil {
			probe.Error = err.Error()
		}
	}

	return probe
}

// probeScript prints one key per line for everything it finds. It always
// exits zero, so a non-zero ssh status means the connection itself failed.
func (s *SSHExecutor) probeScript(become string) string {
	commands := []string{"echo connected"}
	if become != "" {
		commands = append(commands, s.deploymentPHP(become, "-r", "exit(0);")+" >/dev/null 2>&1 && echo deployment-php")
	}
	commands = append(commands,
		"test -d "+shellQuoteArg(path.Dir(s.directory))+" && echo parent-directory",
		"cd "+shellQuoteArg(s.directory)+" 2>/dev/null || exit 0",
		"echo directory",
		s.quotedPHP()+" -r 'exit(0);' >/dev/null 2>&1 && echo php",
		"test -f bin/console && echo console",
		"test -d var/log && echo logs",
		"test -x vendor/bin/shopware-deployment-helper && echo deployment-helper",
		"exit 0",
	)
	return strings.Join(commands, "; ")
}

func parseSSHProbe(output string) SSHProbe {
	var probe SSHProbe

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		switch strings.TrimSpace(scanner.Text()) {
		case "connected":
			probe.Connected = true
		case "parent-directory":
			probe.ParentDirectory = true
		case "directory":
			probe.Directory = true
		case "php":
			probe.PHP = true
		case "deployment-php":
			probe.DeploymentPHP = true
		case "console":
			probe.Console = true
		case "logs":
			probe.Logs = true
		case "deployment-helper":
			probe.DeploymentHelper = true
		}
	}

	return probe
}

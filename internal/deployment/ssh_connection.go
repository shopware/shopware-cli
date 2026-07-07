package deployment

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

type sshConnection struct {
	client *ssh.Client
}

func newSSHConnection(cfg *shop.EnvironmentSSH) (*sshConnection, error) {
	if cfg == nil || cfg.Host == "" {
		return nil, fmt.Errorf("the environment is missing the ssh.host setting")
	}

	port := cfg.Port
	if port == 0 {
		port = 22
	}

	user := cfg.User
	if user == "" {
		user = os.Getenv("USER")
	}

	authMethods, err := sshAuthMethods(cfg)
	if err != nil {
		return nil, err
	}

	hostKeyCallback, err := sshHostKeyCallback(cfg)
	if err != nil {
		return nil, err
	}

	clientConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", port))

	client, err := ssh.Dial("tcp", addr, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to %s: %w", addr, err)
	}

	return &sshConnection{client: client}, nil
}

func sshAuthMethods(cfg *shop.EnvironmentSSH) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if cfg.IdentityFile != "" {
		signer, err := loadPrivateKey(expandHome(cfg.IdentityFile), cfg.Passphrase)
		if err != nil {
			return nil, err
		}

		methods = append(methods, ssh.PublicKeys(signer))
	}

	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}

	if cfg.IdentityFile == "" {
		for _, name := range []string{"id_ed25519", "id_rsa"} {
			keyPath := expandHome(filepath.Join("~", ".ssh", name))
			if _, err := os.Stat(keyPath); err != nil {
				continue
			}

			signer, err := loadPrivateKey(keyPath, cfg.Passphrase)
			if err != nil {
				continue
			}

			methods = append(methods, ssh.PublicKeys(signer))
		}
	}

	if cfg.Password != "" {
		methods = append(methods, ssh.Password(cfg.Password))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no usable SSH authentication found: configure ssh.identity_file or ssh.password, or start an SSH agent")
	}

	return methods, nil
}

func loadPrivateKey(path, passphrase string) (ssh.Signer, error) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read identity file %s: %w", path, err)
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err == nil {
		return signer, nil
	}

	if _, ok := err.(*ssh.PassphraseMissingError); ok {
		if passphrase == "" {
			return nil, fmt.Errorf("identity file %s is passphrase protected, set ssh.passphrase (e.g. via an environment variable)", path)
		}

		signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(passphrase))
		if err != nil {
			return nil, fmt.Errorf("cannot decrypt identity file %s: %w", path, err)
		}

		return signer, nil
	}

	return nil, fmt.Errorf("cannot parse identity file %s: %w", path, err)
}

func sshHostKeyCallback(cfg *shop.EnvironmentSSH) (ssh.HostKeyCallback, error) {
	if cfg.InsecureIgnoreHostKey {
		//nolint:gosec // explicitly requested by the user via insecure_ignore_host_key
		return ssh.InsecureIgnoreHostKey(), nil
	}

	knownHostsFile := cfg.KnownHostsFile
	if knownHostsFile == "" {
		knownHostsFile = filepath.Join("~", ".ssh", "known_hosts")
	}

	knownHostsFile = expandHome(knownHostsFile)

	callback, err := knownhosts.New(knownHostsFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read known_hosts file %s (connect once using ssh, set ssh.known_hosts_file or ssh.insecure_ignore_host_key): %w", knownHostsFile, err)
	}

	return callback, nil
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~"+string(os.PathSeparator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}

		return filepath.Join(home, strings.TrimPrefix(path[1:], string(os.PathSeparator)))
	}

	return path
}

func (c *sshConnection) Run(ctx context.Context, command string) (string, error) {
	logging.FromContext(ctx).Debugf("ssh: %s", command)

	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("cannot open SSH session: %w", err)
	}
	defer func() {
		_ = session.Close()
	}()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return string(output), fmt.Errorf("remote command %q failed: %w\n%s", command, err, strings.TrimSpace(string(output)))
	}

	return string(output), nil
}

func (c *sshConnection) Stream(ctx context.Context, command string, stdin io.Reader) error {
	logging.FromContext(ctx).Debugf("ssh (stream): %s", command)

	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("cannot open SSH session: %w", err)
	}
	defer func() {
		_ = session.Close()
	}()

	session.Stdin = stdin

	output, err := session.CombinedOutput(command)
	if err != nil {
		return fmt.Errorf("remote command %q failed: %w\n%s", command, err, strings.TrimSpace(string(output)))
	}

	return nil
}

func (c *sshConnection) Close() error {
	return c.client.Close()
}

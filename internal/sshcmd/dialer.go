package sshcmd

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
	"time"

	"github.com/shopware/shopware-cli/internal/shop"
)

// Dial opens a raw TCP stream to addr as seen from the remote host, using the
// ssh client's stdio forwarding (ssh -W). Every stream is an ssh channel on
// the multiplexed master connection, so consecutive dials are cheap. The
// returned net.Conn owns the ssh process; closing the connection ends it.
func Dial(cfg *shop.EnvironmentSSH, addr string) (net.Conn, error) {
	args := BaseArgs(cfg)
	args = append(args, "-W", addr, Destination(cfg))

	// the process lifecycle is bound to the connection, not to a dial
	// context: the caller may use the connection long after dialing
	cmd := exec.Command("ssh", args...) //nolint:noctx

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	conn := &pipeConn{cmd: cmd, stdin: stdin, stdout: stdout, addr: addr}
	cmd.Stderr = &conn.stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start ssh for %s: %w", addr, err)
	}

	return conn, nil
}

// pipeConn adapts the stdio of an ssh -W process to a net.Conn.
type pipeConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr bytes.Buffer
	addr   string

	closeOnce sync.Once
}

func (c *pipeConn) Read(p []byte) (int, error) {
	n, err := c.stdout.Read(p)
	if err == io.EOF {
		// the stream ended, usually because the remote side could not be
		// reached; surface the ssh error instead of a bare EOF
		if stderr := bytes.TrimSpace(c.stderr.Bytes()); len(stderr) > 0 {
			return n, fmt.Errorf("ssh stream to %s closed: %s", c.addr, stderr)
		}
	}

	return n, err
}

func (c *pipeConn) Write(p []byte) (int, error) {
	return c.stdin.Write(p)
}

func (c *pipeConn) Close() error {
	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		_ = c.stdout.Close()

		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}

		// the process is terminated deliberately, its exit status carries no
		// signal worth reporting to the caller
		_ = c.cmd.Wait()
	})

	return nil
}

func (c *pipeConn) LocalAddr() net.Addr {
	return sshAddr{addr: "ssh"}
}

func (c *pipeConn) RemoteAddr() net.Addr {
	return sshAddr{addr: c.addr}
}

// The ssh process has no deadline support; the driver only sets deadlines
// when explicit timeouts are configured, which the CLI does not do.
func (c *pipeConn) SetDeadline(time.Time) error      { return nil }
func (c *pipeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *pipeConn) SetWriteDeadline(time.Time) error { return nil }

type sshAddr struct {
	addr string
}

func (a sshAddr) Network() string { return "ssh" }
func (a sshAddr) String() string  { return a.addr }

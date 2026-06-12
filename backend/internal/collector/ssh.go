package collector

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/kubepilot/backend/internal/models"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// defaultKubeconfigCandidates are tried in order when the cluster does not
// specify an explicit remote kubeconfig path. They cover the common single-node
// / control-plane layouts (k3s, kubeadm, plain ~/.kube/config).
var defaultKubeconfigCandidates = []string{
	"/etc/rancher/k3s/k3s.yaml",
	"~/.kube/config",
	"/etc/kubernetes/admin.conf",
}

// sshDialClient opens an SSH connection to the node using password authentication.
func sshDialClient(cluster *models.Cluster) (*ssh.Client, error) {
	port := cluster.SSHPort
	if port == 0 {
		port = 22
	}
	cfg := &ssh.ClientConfig{
		User: cluster.SSHUser,
		Auth: []ssh.AuthMethod{ssh.Password(cluster.SSHPassword)},
		// alpha: host keys are not pinned yet. Trusted-network / bastion use only.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
	addr := fmt.Sprintf("%s:%d", cluster.SSHHost, port)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	return client, nil
}

// sshTunnel manages a single, self-healing SSH connection used to tunnel
// Kubernetes API traffic. The underlying *ssh.Client is recreated transparently
// when it dies (network blip, idle timeout on a restrictive network, etc.), and
// a keepalive keeps it from being reaped while idle between collection passes.
type sshTunnel struct {
	cluster *models.Cluster
	logger  *zap.Logger

	mu     sync.Mutex
	client *ssh.Client
}

// newSSHTunnel establishes the initial SSH connection.
func newSSHTunnel(cluster *models.Cluster, logger *zap.Logger) (*sshTunnel, error) {
	t := &sshTunnel{cluster: cluster, logger: logger}
	if _, err := t.ensureClient(); err != nil {
		return nil, err
	}
	return t, nil
}

// ensureClient returns a live client, dialing a new one (and starting its
// keepalive loop) if none is currently held.
func (t *sshTunnel) ensureClient() (*ssh.Client, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.client != nil {
		return t.client, nil
	}
	c, err := sshDialClient(t.cluster)
	if err != nil {
		return nil, err
	}
	t.client = c
	go t.keepAlive(c)
	return c, nil
}

// invalidate drops the given client if it is still the active one, forcing the
// next ensureClient call to reconnect.
func (t *sshTunnel) invalidate(dead *ssh.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.client == dead {
		t.client = nil
		_ = dead.Close()
	}
}

// Dial routes a TCP connection through the SSH tunnel (matches rest.Config.Dial).
// On failure it assumes the connection died, reconnects once and retries — so a
// dropped tunnel recovers on the next client-go watch/list attempt instead of
// failing forever.
func (t *sshTunnel) Dial(_ context.Context, network, addr string) (net.Conn, error) {
	client, err := t.ensureClient()
	if err != nil {
		return nil, err
	}
	conn, err := client.Dial(network, addr)
	if err == nil {
		return conn, nil
	}

	t.invalidate(client)
	client, err2 := t.ensureClient()
	if err2 != nil {
		return nil, fmt.Errorf("ssh tunnel dial %s (reconnect failed: %v): %w", addr, err2, err)
	}
	return client.Dial(network, addr)
}

// keepAlive pings the SSH server periodically; on failure it invalidates the
// client so the next Dial reconnects. The loop is tied to a specific client and
// exits once that client is replaced.
func (t *sshTunnel) keepAlive(client *ssh.Client) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
			t.logger.Warn("ssh keepalive failed; will reconnect on next use",
				zap.String("ssh_host", t.cluster.SSHHost), zap.Error(err))
			t.invalidate(client)
			return
		}
	}
}

// Close tears down the active SSH connection.
func (t *sshTunnel) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.client == nil {
		return nil
	}
	err := t.client.Close()
	t.client = nil
	return err
}

// fetchRemoteKubeconfig reads the kubeconfig file from the node over SSH. If the
// cluster has an explicit SSHKubeconfigPath it is used (shell-quoted); otherwise
// the common candidate locations are tried in order.
func fetchRemoteKubeconfig(client *ssh.Client, cluster *models.Cluster) ([]byte, error) {
	var exprs []string
	if cluster.SSHKubeconfigPath != "" {
		exprs = append(exprs, shellQuote(cluster.SSHKubeconfigPath))
	} else {
		// Constant, trusted paths — left unquoted so `~` expands on the node.
		exprs = append(exprs, defaultKubeconfigCandidates...)
	}

	var lastErr error
	for _, expr := range exprs {
		data, err := sshReadFile(client, expr, cluster.SSHSudo, cluster.SSHPassword)
		if err == nil && len(bytes.TrimSpace(data)) > 0 {
			return data, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("kubeconfig not found on node (tried %v)", defaultKubeconfigCandidates)
	}
	return nil, lastErr
}

// sshReadFile cats a remote file. pathExpr must already be shell-safe. When
// useSudo is set the command runs via `sudo -S` with the password piped on stdin.
func sshReadFile(client *ssh.Client, pathExpr string, useSudo bool, password string) ([]byte, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	cmd := "cat " + pathExpr
	if useSudo {
		// -S reads the password from stdin; -p '' suppresses the prompt text.
		cmd = "sudo -S -p '' " + cmd
		session.Stdin = strings.NewReader(password + "\n")
	}

	if err := session.Run(cmd); err != nil {
		return nil, fmt.Errorf("run %q: %w (stderr: %s)", cmd, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

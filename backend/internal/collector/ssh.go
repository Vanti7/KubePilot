package collector

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/kubepilot/backend/internal/models"
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

// fetchRemoteKubeconfig reads the kubeconfig file from the node over SSH. If the
// cluster has an explicit SSHKubeconfigPath it is used (shell-quoted); otherwise
// the common candidate locations are tried in order.
func fetchRemoteKubeconfig(client *ssh.Client, cluster *models.Cluster) ([]byte, error) {
	type candidate struct {
		expr string // path token already prepared for the shell
	}

	var candidates []candidate
	if cluster.SSHKubeconfigPath != "" {
		candidates = append(candidates, candidate{expr: shellQuote(cluster.SSHKubeconfigPath)})
	} else {
		for _, p := range defaultKubeconfigCandidates {
			// Constant, trusted paths — left unquoted so `~` expands on the node.
			candidates = append(candidates, candidate{expr: p})
		}
	}

	var lastErr error
	for _, c := range candidates {
		data, err := sshReadFile(client, c.expr, cluster.SSHSudo, cluster.SSHPassword)
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

// sshTunnelDialer returns a dialer (matching rest.Config.Dial) that routes every
// TCP connection through the SSH client. This lets client-go reach an API server
// address (e.g. 127.0.0.1:6443) that is only resolvable from the node itself.
func sshTunnelDialer(client *ssh.Client) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(_ context.Context, network, addr string) (net.Conn, error) {
		return client.Dial(network, addr)
	}
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

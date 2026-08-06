// Package netguard provides an SSRF-resistant HTTP client for outbound
// requests built from data KubePilot does not fully control: image
// references observed in a workload spec, repository URLs returned by
// Artifact Hub search, an operator-supplied Helm repository URL. None of
// these callers require any KubePilot login to trigger a request — the image
// and Helm watchers run on a timer — so the connect itself is the trust
// boundary, not an auth check further up the stack.
package netguard

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// safeDialer refuses to connect to loopback, unspecified, link-local and
// multicast addresses. Control runs after DNS resolution against the actual
// IP about to be dialed, so a DNS answer under attacker control cannot
// redirect a "safe-looking" hostname to a blocked address after the fact
// (DNS rebinding).
//
// RFC1918 private ranges (10/8, 172.16/12, 192.168/16) are deliberately NOT
// blocked: self-hosted registries and chart repositories on a home/office LAN
// (see CLAUDE.md's Harbor CI infra, reachable at 10.0.60.152) are this tool's
// primary use case, not an edge case to defend against.
var safeDialer = &net.Dialer{
	Timeout:   10 * time.Second,
	KeepAlive: 30 * time.Second,
	Control: func(_, address string, c syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("netguard: could not parse address %q", address)
		}
		if blocked, reason := isBlockedIP(ip); blocked {
			return fmt.Errorf("netguard: refusing to connect to %s: %s", ip, reason)
		}
		return nil
	},
}

// isBlockedIP reports whether ip is a loopback, unspecified, link-local
// (including the 169.254.169.254 cloud-metadata endpoint on AWS/GCP/Azure/
// OCI) or multicast address.
func isBlockedIP(ip net.IP) (bool, string) {
	switch {
	case ip.IsLoopback():
		return true, "loopback address"
	case ip.IsUnspecified():
		return true, "unspecified address"
	case ip.IsLinkLocalUnicast():
		return true, "link-local address"
	case ip.IsLinkLocalMulticast(), ip.IsInterfaceLocalMulticast():
		return true, "multicast address"
	}
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 0 {
		return true, "reserved address (0.0.0.0/8)"
	}
	return false, ""
}

// NewHTTPClient returns an http.Client whose transport routes through
// safeDialer. insecureTLS mirrors the existing per-registry/per-repository
// tls_insecure flag (self-signed Harbor & co) — it is never applied to a
// client used for public hosts.
func NewHTTPClient(timeout time.Duration, insecureTLS bool) *http.Client {
	transport := &http.Transport{
		DialContext: safeDialer.DialContext,
	}
	if insecureTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

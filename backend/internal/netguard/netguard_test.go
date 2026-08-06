package netguard

import (
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"loopback v4", "127.0.0.1", true},
		{"loopback v6", "::1", true},
		{"unspecified v4", "0.0.0.0", true},
		{"reserved this-network", "0.5.5.5", true},
		{"cloud metadata", "169.254.169.254", true},
		{"link-local v4", "169.254.1.1", true},
		{"link-local v6", "fe80::1", true},
		{"multicast", "224.0.0.1", true},
		// RFC1918 private ranges must stay reachable — self-hosted registries
		// and chart repos on a home/office LAN are the primary use case (see
		// CLAUDE.md's Harbor CI infra at 10.0.60.152), not a threat to block.
		{"private 10/8", "10.0.60.152", false},
		{"private 172.16/12", "172.16.5.1", false},
		{"private 192.168/16", "192.168.1.1", false},
		{"public", "8.8.8.8", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("failed to parse test IP %q", tc.ip)
			}
			blocked, reason := isBlockedIP(ip)
			if blocked != tc.blocked {
				t.Errorf("isBlockedIP(%s) = %v (%s), want %v", tc.ip, blocked, reason, tc.blocked)
			}
		})
	}
}

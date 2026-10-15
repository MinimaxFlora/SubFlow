package fetch

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",
		"::1",
		"10.0.0.1",
		"172.16.5.4",
		"192.168.1.1",
		"169.254.169.254", // cloud metadata endpoint
		"0.0.0.0",
		"100.64.0.1", // carrier-grade NAT
		"fc00::1",    // IPv6 unique local
		"fe80::1",    // link-local
		"ff02::1",    // multicast
	}
	for _, raw := range blocked {
		if !isBlockedIP(net.ParseIP(raw)) {
			t.Errorf("isBlockedIP(%s) = false, want true", raw)
		}
	}

	allowed := []string{"1.1.1.1", "8.8.8.8", "93.184.216.34", "2606:4700:4700::1111"}
	for _, raw := range allowed {
		if isBlockedIP(net.ParseIP(raw)) {
			t.Errorf("isBlockedIP(%s) = true, want false", raw)
		}
	}
}

func TestValidateURL(t *testing.T) {
	cases := []struct {
		raw       string
		wantError bool
	}{
		{"https://example.com/sub", false},
		{"http://example.com/sub", false},
		{"https://127.0.0.1/sub", true},
		{"https://192.168.0.1/sub", true},
		{"https://169.254.169.254/latest/meta-data/", true},
		{"file:///etc/passwd", true},
		{"ftp://example.com/sub", true},
		{"https://", true},
	}
	for _, tc := range cases {
		u, err := url.Parse(tc.raw)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", tc.raw, err)
		}
		err = validateURL(u, false)
		if tc.wantError && err == nil {
			t.Errorf("validateURL(%q) = nil, want error", tc.raw)
		}
		if !tc.wantError && err != nil {
			t.Errorf("validateURL(%q) = %v, want nil", tc.raw, err)
		}
	}
}

func TestValidateURLAllowsPrivateWhenOptedIn(t *testing.T) {
	u, _ := url.Parse("http://192.168.1.10/sub")
	if err := validateURL(u, true); err != nil {
		t.Fatalf("validateURL with allowPrivate = %v, want nil", err)
	}
}

func TestResolveAllSafeLiteralIP(t *testing.T) {
	ips, err := resolveAllSafe(context.Background(), "1.1.1.1", false)
	if err != nil {
		t.Fatalf("resolveAllSafe: %v", err)
	}
	if len(ips) != 1 || ips[0].String() != "1.1.1.1" {
		t.Fatalf("resolveAllSafe = %v, want [1.1.1.1]", ips)
	}

	if _, err := resolveAllSafe(context.Background(), "127.0.0.1", false); err == nil {
		t.Fatal("resolveAllSafe(127.0.0.1) = nil, want error")
	}
}

// TestDialAnyFallsBackToReachableAddress verifies that a host with an
// unreachable first address is still dialled through a later, reachable one.
func TestDialAnyFallsBackToReachableAddress(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	// A TEST-NET-1 address (RFC 5737) is guaranteed non-routable, so the first
	// attempt fails and the loopback address must be reached instead.
	ips := []net.IP{net.ParseIP("192.0.2.1"), net.ParseIP("127.0.0.1")}
	dialer := &net.Dialer{Timeout: 500 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := dialAny(ctx, dialer, ips, strconv.Itoa(port))
	if err != nil {
		t.Fatalf("dialAny: %v", err)
	}
	defer conn.Close()
}

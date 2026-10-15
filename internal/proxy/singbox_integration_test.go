package proxy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSingBoxAcceptsGeneratedConfig validates the produced sing-box document
// with a real sing-box binary. It is skipped unless SINGBOX_BIN points at one:
//
//	SINGBOX_BIN=/path/to/sing-box go test ./internal/proxy/
func TestSingBoxAcceptsGeneratedConfig(t *testing.T) {
	bin := os.Getenv("SINGBOX_BIN")
	if bin == "" {
		t.Skip("set SINGBOX_BIN to a sing-box binary to run this check")
	}

	links := []string{
		vlessLine,
		trojanLine,
		ssLine,
		vmessLine,
		hy2Line,
		"vless://11111111-2222-3333-4444-555555555555@reality.example.com:443?security=reality&pbk=abcdefghijklmnopqrstuvwxyz1234567890ABCDEFG&sid=ab12&sni=www.microsoft.com&flow=xtls-rprx-vision#REALITY",
	}
	nodes, err := ParseSubscription(strings.Join(links, "\n"))
	if err != nil {
		t.Fatal(err)
	}

	output, err := Produce(nodes, "singbox", Options{})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(output), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "check", "-c", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sing-box rejected the generated config: %v\n%s\nconfig:\n%s", err, out, output)
	}
}

package proxy

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

const (
	vlessLine  = "vless://11111111-2222-3333-4444-555555555555@example.com:443?type=ws&security=tls&sni=example.com&path=%2Fws&host=example.com&fp=chrome#VLESS-WS"
	trojanLine = "trojan://secret@trojan.example.com:443?sni=trojan.example.com#Trojan-Node"
	ssLine     = "ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ@ss.example.com:8388#SS-Node"
	vmessLine  = "vmess://eyJ2IjoiMiIsInBzIjoiVk1lc3MtTm9kZSIsImFkZCI6InZtZXNzLmV4YW1wbGUuY29tIiwicG9ydCI6IjQ0MyIsImlkIjoiMTExMTExMTEtMjIyMi0zMzMzLTQ0NDQtNTU1NTU1NTU1NTU1IiwibmV0Ijoid3MiLCJ0bHMiOiJ0bHMiLCJob3N0Ijoidm1lc3MuZXhhbXBsZS5jb20iLCJwYXRoIjoiL3BhdGgifQ=="
	hy2Line    = "hysteria2://hpass@hy2.example.com:443?sni=hy2.example.com&insecure=1#HY2-Node"
)

func sampleSubscription() string {
	return strings.Join([]string{vlessLine, trojanLine, ssLine, vmessLine, hy2Line}, "\n")
}

func TestParseSampleSubscription(t *testing.T) {
	nodes, err := ParseSubscription(sampleSubscription())
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 5 {
		t.Fatalf("parsed %d nodes, want 5", len(nodes))
	}
	if nodes[0].Type != TypeVLESS || nodes[0].Port != 443 || nodes[0].Security != SecurityTLS {
		t.Fatalf("unexpected vless node: %+v", nodes[0])
	}
	if nodes[2].Type != TypeSS || nodes[2].Cipher != "aes-256-gcm" {
		t.Fatalf("unexpected ss node: %+v", nodes[2])
	}
	if nodes[4].Type != TypeHysteria2 || !nodes[4].SkipCertVerify {
		t.Fatalf("unexpected hysteria2 node: %+v", nodes[4])
	}
}

func TestParseBase64Subscription(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(sampleSubscription()))
	nodes, err := ParseSubscription(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 5 {
		t.Fatalf("parsed %d nodes from base64, want 5", len(nodes))
	}
}

func TestURIRoundTrip(t *testing.T) {
	nodes, err := ParseSubscription(sampleSubscription())
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range nodes {
		uri, err := node.ToURI()
		if err != nil {
			t.Fatalf("emit %s: %v", node.Type, err)
		}
		back, err := ParseURI(uri)
		if err != nil {
			t.Fatalf("reparse %s (%s): %v", node.Type, uri, err)
		}
		if back.Type != node.Type || back.Server != node.Server || back.Port != node.Port {
			t.Fatalf("round trip mismatch: %+v -> %+v", node, back)
		}
	}
}

func TestProduceClashAndSingBoxAreValidDocuments(t *testing.T) {
	nodes, err := ParseSubscription(sampleSubscription())
	if err != nil {
		t.Fatal(err)
	}

	clash, err := Produce(nodes, "clash", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clash, "proxies:") || !strings.Contains(clash, "VLESS-WS") {
		t.Fatalf("clash output looks wrong:\n%s", clash)
	}
	if strings.Contains(clash, "HY2-Node") {
		t.Fatal("clash output should not contain the hysteria2 node")
	}

	singbox, err := Produce(nodes, "singbox", Options{})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(singbox), &doc); err != nil {
		t.Fatalf("sing-box output is not valid JSON: %v", err)
	}
	outbounds, ok := doc["outbounds"].([]any)
	if !ok || len(outbounds) < 5 {
		t.Fatalf("sing-box outbounds missing: %v", doc["outbounds"])
	}
}

func TestOperationsChain(t *testing.T) {
	nodes, err := ParseSubscription(sampleSubscription())
	if err != nil {
		t.Fatal(err)
	}
	out, err := ApplyOperations(nodes, []Operation{
		{Type: "Filter", Args: map[string]any{"regex": "example", "field": "server", "mode": "include"}},
		{Type: "Rename", Args: map[string]any{"prefix": "SF-"}},
		{Type: "Sort", Args: map[string]any{"by": "name", "order": "asc"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 5 {
		t.Fatalf("filter kept %d nodes, want 5", len(out))
	}
	for _, n := range out {
		if !strings.HasPrefix(n.Name, "SF-") {
			t.Fatalf("rename not applied: %q", n.Name)
		}
	}
}

func TestDeduplicateByServer(t *testing.T) {
	nodes := []Node{
		{Name: "a", Type: TypeSS, Server: "1.1.1.1", Port: 80, Cipher: "aes-256-gcm", Password: "x"},
		{Name: "b", Type: TypeSS, Server: "1.1.1.1", Port: 80, Cipher: "aes-256-gcm", Password: "y"},
	}
	out, err := ApplyOperations(nodes, []Operation{{Type: "Deduplicate", Args: map[string]any{"by": "server"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("dedupe produced %d nodes, want 1", len(out))
	}
}

func TestConvertReportsUnsupported(t *testing.T) {
	result, err := Convert(ConvertOptions{Content: sampleSubscription(), Platform: "clash"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 5 || result.Emitted != 4 {
		t.Fatalf("convert stats = total %d emitted %d, want 5/4", result.Total, result.Emitted)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected a warning about omitted nodes")
	}
}

// sing-box requires uTLS for a REALITY client, so the producer must inject a
// fingerprint even when the source node does not specify one.
func TestSingBoxRealityAlwaysHasUTLS(t *testing.T) {
	link := "vless://11111111-2222-3333-4444-555555555555@reality.example.com:443?security=reality&pbk=abcdefghijklmnopqrstuvwxyz1234567890ABCDEFG&sid=ab12&sni=www.microsoft.com#REALITY"
	nodes, err := ParseSubscription(link)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Produce(nodes, "singbox", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"utls"`) || !strings.Contains(out, `"reality"`) {
		t.Fatalf("sing-box reality outbound missing utls: %s", out)
	}
}

package proxy

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// produceClash renders a Clash / Mihomo / Stash compatible document.
func produceClash(nodes []Node, platform string, opts Options) (string, error) {
	proxies := make([]map[string]any, 0, len(nodes))
	names := make([]string, 0, len(nodes))
	for _, n := range nodes {
		entry, err := clashProxy(platform, n)
		if err != nil {
			return "", err
		}
		proxies = append(proxies, entry)
		names = append(names, n.Name)
	}

	doc := map[string]any{
		"proxies": proxies,
	}
	if len(names) > 0 {
		group := map[string]any{
			"name":    "SubFlow",
			"type":    "select",
			"proxies": append([]string{"DIRECT", "REJECT"}, names...),
		}
		doc["proxy-groups"] = []map[string]any{group}
		doc["rules"] = []string{"MATCH,SubFlow"}
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal clash yaml: %w", err)
	}
	return string(raw), nil
}

// clashProxy renders one node. platform decides which extensions are emitted:
// Clash.Meta (mihomo) and Stash understand REALITY and the QUIC-based
// protocols, while the original Clash does not.
func clashProxy(platform string, n Node) (map[string]any, error) {
	base := map[string]any{
		"name":   n.Name,
		"server": n.Server,
		"port":   n.Port,
	}
	switch n.Type {
	case TypeSS:
		base["type"] = "ss"
		base["cipher"] = n.Cipher
		base["password"] = n.Password
	case TypeVMess:
		base["type"] = "vmess"
		base["uuid"] = n.UUID
		base["alterId"] = n.AlterID
		if n.Cipher != "" {
			base["cipher"] = n.Cipher
		}
		applyClashTLS(base, n)
		applyClashTransport(base, n)
	case TypeVLESS:
		base["type"] = "vless"
		base["uuid"] = n.UUID
		if n.Flow != "" {
			base["flow"] = n.Flow
		}
		applyClashTLS(base, n)
		applyClashTransport(base, n)
	case TypeTrojan:
		base["type"] = "trojan"
		base["password"] = n.Password
		if n.Security != SecurityNone {
			base["tls"] = true
		}
		if n.SNI != "" {
			base["sni"] = n.SNI
		}
		if n.SkipCertVerify {
			base["skip-cert-verify"] = true
		}
		applyClashTransport(base, n)
	case TypeHysteria2:
		if !supportsQUIC(platform) {
			return nil, fmt.Errorf("clash: %s does not support hysteria2", platform)
		}
		base["type"] = "hysteria2"
		base["password"] = n.Password
		if n.SNI != "" {
			base["sni"] = n.SNI
		}
		if n.SkipCertVerify {
			base["skip-cert-verify"] = true
		}
		if len(n.ALPN) > 0 {
			base["alpn"] = n.ALPN
		}
		if n.Obfs != "" {
			base["obfs"] = n.Obfs
			if n.ObfsPassword != "" {
				base["obfs-password"] = n.ObfsPassword
			}
		}
	case TypeTUIC:
		if !supportsQUIC(platform) {
			return nil, fmt.Errorf("clash: %s does not support tuic", platform)
		}
		base["type"] = "tuic"
		base["uuid"] = n.UUID
		base["password"] = n.Password
		if n.SNI != "" {
			base["sni"] = n.SNI
		}
		if n.SkipCertVerify {
			base["skip-cert-verify"] = true
		}
		if n.Congestion != "" {
			base["congestion-controller"] = n.Congestion
		}
		if n.UDPRelayMode != "" {
			base["udp-relay-mode"] = n.UDPRelayMode
		}
		if len(n.ALPN) > 0 {
			base["alpn"] = n.ALPN
		}
	default:
		return nil, fmt.Errorf("clash: unsupported type %q", n.Type)
	}
	return base, nil
}

// supportsQUIC reports whether a Clash-family target understands hysteria2/tuic.
func supportsQUIC(platform string) bool {
	return platform == "clashmeta" || platform == "stash"
}

func applyClashTLS(base map[string]any, n Node) {
	if n.Security == SecurityTLS {
		base["tls"] = true
	}
	if n.Security == SecurityReality {
		base["tls"] = true
		base["reality-opts"] = map[string]any{
			"public-key": n.RealityPublic,
			"short-id":   n.RealityShortID,
		}
	}
	if n.SNI != "" {
		base["servername"] = n.SNI
	}
	if n.Fingerprint != "" {
		base["client-fingerprint"] = n.Fingerprint
	}
	if n.SkipCertVerify {
		base["skip-cert-verify"] = true
	}
	if len(n.ALPN) > 0 {
		base["alpn"] = n.ALPN
	}
}

func applyClashTransport(base map[string]any, n Node) {
	switch n.Network {
	case NetworkWS:
		base["network"] = "ws"
		opts := map[string]any{"path": firstNonEmpty(n.Path, "/")}
		if n.Host != "" {
			opts["headers"] = map[string]string{"Host": n.Host}
		}
		base["ws-opts"] = opts
	case NetworkGRPC:
		base["network"] = "grpc"
		base["grpc-opts"] = map[string]any{"grpc-service-name": n.ServiceName}
	case NetworkH2:
		base["network"] = "h2"
		opts := map[string]any{"path": firstNonEmpty(n.Path, "/")}
		if n.Host != "" {
			opts["host"] = []string{n.Host}
		}
		base["h2-opts"] = opts
	}
}

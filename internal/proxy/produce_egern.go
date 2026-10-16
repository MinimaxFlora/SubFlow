package proxy

import (
	"encoding/json"
	"fmt"
	"strings"
)

// produceEgern renders nodes for the Egern client. Egern accepts a YAML list of
// proxies where each entry is a JSON object; the upstream producer emits the
// same `proxies:\n  - {json}` shape, which is what this mirrors.
func produceEgern(nodes []Node) (string, error) {
	var sb strings.Builder
	sb.WriteString("proxies:\n")
	for _, n := range nodes {
		entry, err := egernProxy(n)
		if err != nil {
			return "", err
		}
		raw, err := json.Marshal(entry)
		if err != nil {
			return "", fmt.Errorf("marshal egern proxy %q: %w", n.Name, err)
		}
		sb.WriteString("  - ")
		sb.Write(raw)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// egernProxy maps a node onto Egern's proxy schema for the protocols that this
// model supports.
func egernProxy(n Node) (map[string]any, error) {
	base := map[string]any{
		"name":   n.Name,
		"server": n.Server,
		"port":   n.Port,
	}

	switch n.Type {
	case TypeSS:
		base["type"] = "ss"
		base["method"] = n.Cipher
		base["password"] = n.Password

	case TypeTrojan:
		base["type"] = "trojan"
		base["password"] = n.Password
		base["skip_tls_verify"] = n.SkipCertVerify
		transport := egernTransport(n)
		base["transport"] = transport
		if n.SNI != "" {
			base["sni"] = n.SNI
		}

	case TypeVLESS:
		base["type"] = "vless"
		base["user_id"] = n.UUID
		if n.Flow != "" {
			base["flow"] = n.Flow
		}
		base["transport"] = egernTransport(n)

	case TypeVMess:
		base["type"] = "vmess"
		base["user_id"] = n.UUID
		base["security"] = vmessSecurity(n)
		base["transport"] = egernTransport(n)

	case TypeHysteria2:
		base["type"] = "hysteria2"
		base["password"] = n.Password
		if n.Obfs != "" {
			base["obfs"] = n.Obfs
			base["obfs_password"] = n.ObfsPassword
		}
		if n.SNI != "" {
			base["sni"] = n.SNI
		}
		base["skip_tls_verify"] = n.SkipCertVerify

	case TypeTUIC:
		base["type"] = "tuic"
		base["user_id"] = n.UUID
		if n.Password != "" {
			base["password"] = n.Password
		}
		if n.Congestion != "" {
			base["congestion_control"] = n.Congestion
		}
		if n.UDPRelayMode != "" {
			base["udp_relay_mode"] = n.UDPRelayMode
		}
		if n.SNI != "" {
			base["sni"] = n.SNI
		}
		base["skip_tls_verify"] = n.SkipCertVerify

	default:
		return nil, fmt.Errorf("node %q: egern does not support type %q", n.Name, n.Type)
	}

	return base, nil
}

// egernTransport renders the transport block. Egern keys the transport by
// protocol name (tcp/tls/ws/wss/http1/http2/grpc), matching upstream.
func egernTransport(n Node) map[string]any {
	switch n.Network {
	case NetworkWS:
		key := "ws"
		if n.HasTLS() {
			key = "wss"
		}
		inner := map[string]any{}
		if n.Path != "" {
			inner["path"] = n.Path
		}
		if n.Host != "" {
			inner["headers"] = map[string]any{"Host": n.Host}
		}
		if n.HasTLS() {
			inner["sni"] = n.EffectiveSNI()
			inner["skip_tls_verify"] = n.SkipCertVerify
		}
		return map[string]any{key: inner}

	case NetworkHTTP:
		inner := map[string]any{}
		if n.Path != "" {
			inner["path"] = n.Path
		}
		if n.Host != "" {
			inner["headers"] = map[string]any{"Host": n.Host}
		}
		inner["skip_tls_verify"] = n.SkipCertVerify
		return map[string]any{"http1": inner}

	case NetworkH2:
		inner := map[string]any{}
		if n.Path != "" {
			inner["path"] = n.Path
		}
		if n.Host != "" {
			inner["headers"] = map[string]any{"Host": n.Host}
		}
		inner["sni"] = n.EffectiveSNI()
		inner["skip_tls_verify"] = n.SkipCertVerify
		return map[string]any{"http2": inner}

	case NetworkGRPC:
		inner := map[string]any{}
		if n.ServiceName != "" {
			inner["service_name"] = n.ServiceName
		}
		if n.Host != "" {
			inner["headers"] = map[string]any{"Host": n.Host}
		}
		if n.HasTLS() {
			inner["sni"] = n.EffectiveSNI()
			inner["skip_tls_verify"] = n.SkipCertVerify
		}
		return map[string]any{"grpc": inner}

	default: // tcp with or without TLS
		inner := map[string]any{}
		if n.HasTLS() {
			inner["sni"] = n.EffectiveSNI()
			inner["skip_tls_verify"] = n.SkipCertVerify
		}
		if n.Security == SecurityReality {
			reality := map[string]any{}
			if n.RealityPublic != "" {
				reality["public_key"] = n.RealityPublic
			}
			if n.RealityShortID != "" {
				reality["short_id"] = n.RealityShortID
			}
			if len(reality) > 0 {
				inner["reality"] = reality
			}
		}
		key := "tcp"
		if n.HasTLS() {
			key = "tls"
		}
		return map[string]any{key: inner}
	}
}

// vmessSecurity returns the effective VMess cipher, defaulting to auto.
func vmessSecurity(n Node) string {
	if n.Cipher != "" {
		return n.Cipher
	}
	return "auto"
}

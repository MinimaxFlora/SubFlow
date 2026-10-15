package proxy

import (
	"encoding/json"
	"fmt"
)

// produceSingBox renders a sing-box outbounds document.
func produceSingBox(nodes []Node, opts Options) (string, error) {
	outbounds := make([]map[string]any, 0, len(nodes)+1)
	outbounds = append(outbounds, map[string]any{"type": "direct", "tag": "direct"})

	tags := make([]string, 0, len(nodes))
	for _, n := range nodes {
		entry, err := singBoxOutbound(n)
		if err != nil {
			if opts.IncludeUnsupported {
				return "", err
			}
			continue
		}
		outbounds = append(outbounds, entry)
		tags = append(tags, n.Name)
	}

	doc := map[string]any{
		"outbounds": outbounds,
	}
	if len(tags) > 1 {
		doc["outbounds"] = append(outbounds, map[string]any{
			"type":      "selector",
			"tag":       "select",
			"outbounds": append([]string{"direct"}, tags...),
		})
	}

	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal sing-box json: %w", err)
	}
	return string(raw), nil
}

func singBoxOutbound(n Node) (map[string]any, error) {
	base := map[string]any{
		"tag":         n.Name,
		"server":      n.Server,
		"server_port": n.Port,
	}
	switch n.Type {
	case TypeSS:
		base["type"] = "shadowsocks"
		base["method"] = n.Cipher
		base["password"] = n.Password
	case TypeVMess:
		base["type"] = "vmess"
		base["uuid"] = n.UUID
		base["alter_id"] = n.AlterID
		if n.Cipher != "" {
			base["security"] = n.Cipher
		}
		applySingBoxTLS(base, n)
		applySingBoxTransport(base, n)
	case TypeVLESS:
		base["type"] = "vless"
		base["uuid"] = n.UUID
		if n.Flow != "" {
			base["flow"] = n.Flow
		}
		applySingBoxTLS(base, n)
		applySingBoxTransport(base, n)
	case TypeTrojan:
		base["type"] = "trojan"
		base["password"] = n.Password
		if n.SNI != "" {
			base["tls"] = map[string]any{"enabled": true, "server_name": n.SNI, "insecure": n.SkipCertVerify}
		}
		applySingBoxTransport(base, n)
	case TypeHysteria2:
		base["type"] = "hysteria2"
		base["password"] = n.Password
		tls := map[string]any{"enabled": true, "insecure": n.SkipCertVerify}
		if n.SNI != "" {
			tls["server_name"] = n.SNI
		}
		if len(n.ALPN) > 0 {
			tls["alpn"] = n.ALPN
		}
		base["tls"] = tls
		if n.Obfs != "" {
			base["obfs"] = map[string]any{"type": n.Obfs, "password": n.ObfsPassword}
		}
	case TypeTUIC:
		base["type"] = "tuic"
		base["uuid"] = n.UUID
		base["password"] = n.Password
		if n.Congestion != "" {
			base["congestion_control"] = n.Congestion
		}
		if n.UDPRelayMode != "" {
			base["udp_relay_mode"] = n.UDPRelayMode
		}
		tls := map[string]any{"enabled": true, "insecure": n.SkipCertVerify}
		if n.SNI != "" {
			tls["server_name"] = n.SNI
		}
		if len(n.ALPN) > 0 {
			tls["alpn"] = n.ALPN
		}
		base["tls"] = tls
	default:
		return nil, fmt.Errorf("sing-box: unsupported type %q", n.Type)
	}
	return base, nil
}

func applySingBoxTLS(base map[string]any, n Node) {
	if n.Security == SecurityNone {
		return
	}
	tls := map[string]any{"enabled": true, "insecure": n.SkipCertVerify}
	if n.SNI != "" {
		tls["server_name"] = n.SNI
	}
	if len(n.ALPN) > 0 {
		tls["alpn"] = n.ALPN
	}
	if n.Security == SecurityReality {
		// sing-box rejects a REALITY client without uTLS, so make sure a
		// fingerprint is always present, defaulting to chrome.
		fingerprint := n.Fingerprint
		if fingerprint == "" {
			fingerprint = "chrome"
		}
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": fingerprint}
		tls["reality"] = map[string]any{
			"enabled":    true,
			"public_key": n.RealityPublic,
			"short_id":   n.RealityShortID,
		}
	} else if n.Fingerprint != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": n.Fingerprint}
	}
	base["tls"] = tls
}

func applySingBoxTransport(base map[string]any, n Node) {
	switch n.Network {
	case NetworkWS:
		opts := map[string]any{"type": "ws", "path": firstNonEmpty(n.Path, "/")}
		if n.Host != "" {
			opts["headers"] = map[string]string{"Host": n.Host}
		}
		base["transport"] = opts
	case NetworkGRPC:
		base["transport"] = map[string]any{"type": "grpc", "service_name": n.ServiceName}
	case NetworkH2, NetworkHTTP:
		base["transport"] = map[string]any{"type": "http", "host": []string{n.Host}, "path": firstNonEmpty(n.Path, "/")}
	}
}

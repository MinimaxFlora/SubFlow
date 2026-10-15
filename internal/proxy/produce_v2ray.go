package proxy

import (
	"encoding/json"
	"fmt"
)

// produceV2Ray renders a V2Ray outbound JSON document.
func produceV2Ray(nodes []Node, opts Options) (string, error) {
	outbounds := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		entry, err := v2rayOutbound(n)
		if err != nil {
			if opts.IncludeUnsupported {
				return "", err
			}
			continue
		}
		outbounds = append(outbounds, entry)
	}
	doc := map[string]any{
		"outbounds": outbounds,
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal v2ray json: %w", err)
	}
	return string(raw), nil
}

func v2rayOutbound(n Node) (map[string]any, error) {
	out := map[string]any{
		"tag":      n.Name,
		"protocol": n.Type,
		"settings": v2raySettings(n),
	}
	stream := v2rayStream(n)
	if len(stream) > 0 {
		out["streamSettings"] = stream
	}
	switch n.Type {
	case TypeVLESS, TypeVMess, TypeTrojan, TypeSS, TypeHysteria2, TypeTUIC:
		return out, nil
	default:
		return nil, fmt.Errorf("v2ray: unsupported type %q", n.Type)
	}
}

func v2raySettings(n Node) map[string]any {
	server := map[string]any{"address": n.Server, "port": n.Port}
	switch n.Type {
	case TypeVMess:
		server["users"] = []map[string]any{{
			"id":       n.UUID,
			"alterId":  n.AlterID,
			"security": firstNonEmpty(n.Cipher, "auto"),
		}}
	case TypeVLESS:
		user := map[string]any{"id": n.UUID, "encryption": "none"}
		if n.Flow != "" {
			user["flow"] = n.Flow
		}
		server["users"] = []map[string]any{user}
	case TypeTrojan:
		server["servers"] = []map[string]any{{"address": n.Server, "port": n.Port, "password": n.Password}}
		return map[string]any{"servers": server["servers"]}
	case TypeSS:
		server["method"] = n.Cipher
		server["password"] = n.Password
	}
	return map[string]any{"vnext": []map[string]any{server}}
}

func v2rayStream(n Node) map[string]any {
	stream := map[string]any{}
	switch n.Network {
	case NetworkWS:
		ws := map[string]any{"path": firstNonEmpty(n.Path, "/")}
		if n.Host != "" {
			ws["headers"] = map[string]string{"Host": n.Host}
		}
		stream["network"] = "ws"
		stream["wsSettings"] = ws
	case NetworkGRPC:
		stream["network"] = "grpc"
		stream["grpcSettings"] = map[string]any{"serviceName": n.ServiceName}
	case NetworkH2:
		stream["network"] = "h2"
		stream["httpSettings"] = map[string]any{"host": []string{n.Host}, "path": firstNonEmpty(n.Path, "/")}
	}
	if n.HasTLS() {
		tls := map[string]any{"allowInsecure": n.SkipCertVerify}
		if n.EffectiveSNI() != "" {
			tls["serverName"] = n.EffectiveSNI()
		}
		if len(n.ALPN) > 0 {
			tls["alpn"] = n.ALPN
		}
		if n.Fingerprint != "" {
			tls["fingerprint"] = n.Fingerprint
		}
		if n.Security == SecurityReality {
			tls["realitySettings"] = map[string]any{
				"publicKey": n.RealityPublic,
				"shortId":   n.RealityShortID,
			}
		}
		stream["security"] = firstNonEmpty(n.Security, "tls")
		stream["tlsSettings"] = tls
	}
	return stream
}

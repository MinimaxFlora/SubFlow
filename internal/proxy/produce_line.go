package proxy

import (
	"fmt"
	"strconv"
	"strings"
)

// produceSurge renders Surge / Surge Mac policy lines.
func produceSurge(nodes []Node) string {
	lines := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if line, ok := surgeLine(n); ok {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func surgeLine(n Node) (string, bool) {
	parts := []string{}
	switch n.Type {
	case TypeSS:
		parts = append(parts, "ss", n.Server, strconv.Itoa(n.Port),
			"encrypt-method="+n.Cipher, "password="+n.Password)
	case TypeVMess:
		parts = append(parts, "vmess", n.Server, strconv.Itoa(n.Port), "username="+n.UUID)
		parts = append(parts, surgeTLS(n)...)
		parts = append(parts, surgeTransport(n)...)
	case TypeTrojan:
		parts = append(parts, "trojan", n.Server, strconv.Itoa(n.Port), "password="+n.Password)
		parts = append(parts, surgeTLS(n)...)
		parts = append(parts, surgeTransport(n)...)
	case TypeVLESS:
		parts = append(parts, "vless", n.Server, strconv.Itoa(n.Port), "username="+n.UUID)
		if n.Flow != "" {
			parts = append(parts, "flow="+n.Flow)
		}
		parts = append(parts, surgeTLS(n)...)
		parts = append(parts, surgeTransport(n)...)
	case TypeHysteria2:
		parts = append(parts, "hysteria2", n.Server, strconv.Itoa(n.Port), "password="+n.Password)
		if n.SNI != "" {
			parts = append(parts, "sni="+n.SNI)
		}
		if n.Obfs != "" {
			parts = append(parts, "obfs="+n.Obfs)
		}
	case TypeTUIC:
		parts = append(parts, "tuic", n.Server, strconv.Itoa(n.Port), "token="+n.UUID+":"+n.Password)
		if n.SNI != "" {
			parts = append(parts, "sni="+n.SNI)
		}
	default:
		return "", false
	}
	parts = append(parts, "udp-relay=true")
	return fmt.Sprintf("%s = %s", n.Name, strings.Join(parts, ", ")), true
}

func surgeTLS(n Node) []string {
	out := []string{}
	switch n.Security {
	case SecurityTLS, SecurityReality:
		out = append(out, "tls=true")
	}
	if n.EffectiveSNI() != "" {
		out = append(out, "sni="+n.EffectiveSNI())
	}
	if n.SkipCertVerify {
		out = append(out, "skip-cert-verify=true")
	}
	if n.RealityPublic != "" {
		out = append(out, "reality-public-key="+n.RealityPublic)
	}
	return out
}

func surgeTransport(n Node) []string {
	switch n.Network {
	case NetworkWS:
		out := []string{"ws=true", "ws-path=" + firstNonEmpty(n.Path, "/")}
		if n.Host != "" {
			out = append(out, "ws-headers=Host:"+n.Host)
		}
		return out
	case NetworkGRPC:
		return []string{"grpc=true", "grpc-service-name=" + n.ServiceName}
	default:
		return nil
	}
}

// produceLoon renders Loon node lines.
func produceLoon(nodes []Node) string {
	lines := make([]string, 0, len(nodes))
	for _, n := range nodes {
		lines = append(lines, loonLine(n))
	}
	return strings.Join(lines, "\n")
}

func loonLine(n Node) string {
	parts := []string{}
	switch n.Type {
	case TypeSS:
		parts = append(parts, "Shadowsocks", n.Server, strconv.Itoa(n.Port), n.Cipher, n.Password)
	case TypeVMess:
		parts = append(parts, "Vmess", n.Server, strconv.Itoa(n.Port), "aes-128-gcm", n.UUID)
		parts = append(parts, loonTLS(n)...)
		parts = append(parts, loonTransport(n)...)
	case TypeTrojan:
		parts = append(parts, "Trojan", n.Server, strconv.Itoa(n.Port), n.Password)
		parts = append(parts, loonTLS(n)...)
		parts = append(parts, loonTransport(n)...)
	case TypeVLESS:
		parts = append(parts, "Vless", n.Server, strconv.Itoa(n.Port), n.UUID)
		if n.Flow != "" {
			parts = append(parts, "flow:"+n.Flow)
		}
		parts = append(parts, loonTLS(n)...)
		parts = append(parts, loonTransport(n)...)
	case TypeHysteria2:
		parts = append(parts, "Hysteria2", n.Server, strconv.Itoa(n.Port), n.Password)
		if n.SNI != "" {
			parts = append(parts, "tls-name:"+n.SNI)
		}
	case TypeTUIC:
		parts = append(parts, "TUIC", n.Server, strconv.Itoa(n.Port), n.UUID, n.Password)
		if n.SNI != "" {
			parts = append(parts, "tls-name:"+n.SNI)
		}
	}
	return fmt.Sprintf("%s = %s", n.Name, strings.Join(parts, ","))
}

func loonTLS(n Node) []string {
	out := []string{}
	if n.Security == SecurityTLS || n.Security == SecurityReality {
		out = append(out, "over-tls:true")
	}
	if n.EffectiveSNI() != "" {
		out = append(out, "tls-name:"+n.EffectiveSNI())
	}
	if n.SkipCertVerify {
		out = append(out, "skip-cert-verify:true")
	}
	if n.RealityPublic != "" {
		out = append(out, "reality-pubkey:"+n.RealityPublic)
	}
	return out
}

func loonTransport(n Node) []string {
	switch n.Network {
	case NetworkWS:
		out := []string{"transport:ws", "path:" + firstNonEmpty(n.Path, "/")}
		if n.Host != "" {
			out = append(out, "host:"+n.Host)
		}
		return out
	case NetworkGRPC:
		return []string{"transport:grpc", "service-name:" + n.ServiceName}
	default:
		return nil
	}
}

// produceQX renders Quantumult X server lines.
func produceQX(nodes []Node) string {
	lines := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if line, ok := qxLine(n); ok {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func qxLine(n Node) (string, bool) {
	address := fmt.Sprintf("%s:%d", n.Server, n.Port)
	common := func(method string) []string {
		parts := []string{method, "password=" + n.Password}
		return parts
	}
	var kind string
	var parts []string
	switch n.Type {
	case TypeSS:
		kind = "shadowsocks"
		parts = common(n.Cipher)
	case TypeVMess:
		kind = "vmess"
		parts = []string{"method=" + firstNonEmpty(n.Cipher, "chacha20-ietf-poly1305"), "password=" + n.UUID}
		if n.Network == NetworkWS {
			parts = append(parts, "obfs=ws", "obfs-uri="+firstNonEmpty(n.Path, "/"))
			if n.Host != "" {
				parts = append(parts, "obfs-host="+n.Host)
			}
		}
	case TypeVLESS:
		kind = "vless"
		parts = []string{"method=none", "password=" + n.UUID}
		if n.Flow != "" {
			parts = append(parts, "flow="+n.Flow)
		}
		if n.Network == NetworkWS {
			parts = append(parts, "obfs=ws", "obfs-uri="+firstNonEmpty(n.Path, "/"))
			if n.Host != "" {
				parts = append(parts, "obfs-host="+n.Host)
			}
		}
	case TypeTrojan:
		kind = "trojan"
		parts = []string{"password=" + n.Password, "over-tls=true"}
	case TypeHysteria2:
		kind = "hysteria2"
		parts = []string{"password=" + n.Password}
	default:
		return "", false
	}
	if n.HasTLS() {
		parts = append(parts, "tls-verification="+strconv.FormatBool(!n.SkipCertVerify), "tls-host="+n.EffectiveSNI())
	}
	parts = append(parts, "tag="+n.Name)
	return fmt.Sprintf("%s=%s, %s", kind, address, strings.Join(parts, ", ")), true
}

// produceSurfboard renders Surfboard lines (Surge-compatible subset).
func produceSurfboard(nodes []Node) string {
	lines := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if line, ok := surgeLine(n); ok {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

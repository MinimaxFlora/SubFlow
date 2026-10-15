package proxy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ToURI serialises a node back into a share link.
func (n Node) ToURI() (string, error) {
	switch n.Type {
	case TypeVLESS:
		return n.vlessURI(), nil
	case TypeTrojan:
		return n.trojanURI(), nil
	case TypeSS:
		return n.ssURI(), nil
	case TypeVMess:
		return n.vmessURI(), nil
	case TypeHysteria2:
		return n.hysteria2URI(), nil
	case TypeTUIC:
		return n.tuicURI(), nil
	default:
		return "", fmt.Errorf("cannot emit uri for type %q", n.Type)
	}
}

func (n Node) vlessURI() string {
	q := url.Values{}
	q.Set("type", networkOrDefault(n.Network))
	if n.Security != SecurityNone {
		q.Set("security", n.Security)
	}
	if n.Flow != "" {
		q.Set("flow", n.Flow)
	}
	if n.SNI != "" {
		q.Set("sni", n.SNI)
	}
	if n.Fingerprint != "" {
		q.Set("fp", n.Fingerprint)
	}
	if len(n.ALPN) > 0 {
		q.Set("alpn", strings.Join(n.ALPN, ","))
	}
	if n.SkipCertVerify {
		q.Set("allowInsecure", "1")
	}
	if n.RealityPublic != "" {
		q.Set("pbk", n.RealityPublic)
	}
	if n.RealityShortID != "" {
		q.Set("sid", n.RealityShortID)
	}
	applyTransportQuery(q, n)
	return "vless://" + n.UUID + "@" + n.serverPort() + "?" + q.Encode() + nameFragment(n.Name)
}

func (n Node) trojanURI() string {
	q := url.Values{}
	q.Set("type", networkOrDefault(n.Network))
	if n.Security == SecurityNone {
		q.Set("security", "none")
	}
	if n.SNI != "" {
		q.Set("sni", n.SNI)
	}
	if n.Fingerprint != "" {
		q.Set("fp", n.Fingerprint)
	}
	if len(n.ALPN) > 0 {
		q.Set("alpn", strings.Join(n.ALPN, ","))
	}
	if n.SkipCertVerify {
		q.Set("allowInsecure", "1")
	}
	applyTransportQuery(q, n)
	return "trojan://" + url.QueryEscape(n.Password) + "@" + n.serverPort() + "?" + q.Encode() + nameFragment(n.Name)
}

func (n Node) ssURI() string {
	userinfo := n.Cipher + ":" + n.Password
	encoded := base64.RawURLEncoding.EncodeToString([]byte(userinfo))
	link := "ss://" + encoded + "@" + n.serverPort()
	if n.Plugin != "" {
		q := url.Values{}
		q.Set("plugin", n.Plugin)
		link += "?" + q.Encode()
	}
	return link + nameFragment(n.Name)
}

func (n Node) vmessURI() string {
	payload := map[string]any{
		"v":    "2",
		"ps":   n.Name,
		"add":  n.Server,
		"port": strconv.Itoa(n.Port),
		"id":   n.UUID,
		"aid":  strconv.Itoa(n.AlterID),
		"net":  networkOrDefault(n.Network),
		"type": "none",
		"host": n.Host,
		"path": n.Path,
		"tls":  tlsFlag(n.Security),
	}
	if n.Cipher != "" {
		payload["scy"] = n.Cipher
	}
	if n.SNI != "" {
		payload["sni"] = n.SNI
	}
	if len(n.ALPN) > 0 {
		payload["alpn"] = strings.Join(n.ALPN, ",")
	}
	if n.Fingerprint != "" {
		payload["fp"] = n.Fingerprint
	}
	raw, _ := json.Marshal(payload)
	return "vmess://" + base64.StdEncoding.EncodeToString(raw)
}

func (n Node) hysteria2URI() string {
	q := url.Values{}
	if n.SNI != "" {
		q.Set("sni", n.SNI)
	}
	if n.SkipCertVerify {
		q.Set("insecure", "1")
	}
	if n.Obfs != "" {
		q.Set("obfs", n.Obfs)
	}
	if n.ObfsPassword != "" {
		q.Set("obfs-password", n.ObfsPassword)
	}
	if len(n.ALPN) > 0 {
		q.Set("alpn", strings.Join(n.ALPN, ","))
	}
	link := "hysteria2://" + url.QueryEscape(n.Password) + "@" + n.serverPort()
	if len(q) > 0 {
		link += "?" + q.Encode()
	}
	return link + nameFragment(n.Name)
}

func (n Node) tuicURI() string {
	q := url.Values{}
	if n.SNI != "" {
		q.Set("sni", n.SNI)
	}
	if len(n.ALPN) > 0 {
		q.Set("alpn", strings.Join(n.ALPN, ","))
	}
	if n.SkipCertVerify {
		q.Set("allow_insecure", "1")
	}
	if n.Congestion != "" {
		q.Set("congestion_control", n.Congestion)
	}
	if n.UDPRelayMode != "" {
		q.Set("udp_relay_mode", n.UDPRelayMode)
	}
	link := "tuic://" + n.UUID + ":" + url.QueryEscape(n.Password) + "@" + n.serverPort()
	if len(q) > 0 {
		link += "?" + q.Encode()
	}
	return link + nameFragment(n.Name)
}

func applyTransportQuery(q url.Values, n Node) {
	switch n.Network {
	case NetworkWS:
		q.Set("path", firstNonEmpty(n.Path, "/"))
		if n.Host != "" {
			q.Set("host", n.Host)
		}
	case NetworkGRPC:
		if n.ServiceName != "" {
			q.Set("serviceName", n.ServiceName)
		}
	case NetworkH2, NetworkHTTP:
		q.Set("path", firstNonEmpty(n.Path, "/"))
		if n.Host != "" {
			q.Set("host", n.Host)
		}
	}
}

func networkOrDefault(network string) string {
	if network == "" {
		return NetworkTCP
	}
	return network
}

func tlsFlag(security string) string {
	if security == SecurityTLS || security == SecurityReality {
		return "tls"
	}
	return ""
}

func nameFragment(name string) string {
	if name == "" {
		return ""
	}
	return "#" + url.QueryEscape(name)
}

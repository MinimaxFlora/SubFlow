// Package proxy implements a unified proxy node model together with parsing,
// operations and multi-platform output producers.
package proxy

import (
	"fmt"
	"strconv"
	"strings"
)

// Supported proxy protocols.
const (
	TypeVLESS     = "vless"
	TypeVMess     = "vmess"
	TypeTrojan    = "trojan"
	TypeSS        = "shadowsocks"
	TypeHysteria2 = "hysteria2"
	TypeTUIC      = "tuic"
)

// Supported transport networks.
const (
	NetworkTCP  = "tcp"
	NetworkWS   = "ws"
	NetworkGRPC = "grpc"
	NetworkH2   = "h2"
	NetworkHTTP = "http"
)

// TLS security modes.
const (
	SecurityNone    = ""
	SecurityTLS     = "tls"
	SecurityReality = "reality"
)

// Node is the unified representation of a single proxy node. It keeps the
// common fields explicit and preserves unrecognised values in Extra so that
// conversion never silently drops protocol parameters.
type Node struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Server string `json:"server"`
	Port   int    `json:"port"`

	// Credentials
	UUID     string `json:"uuid,omitempty"`
	Password string `json:"password,omitempty"`
	AlterID  int    `json:"alterId,omitempty"`
	Cipher   string `json:"cipher,omitempty"`
	Flow     string `json:"flow,omitempty"`

	// TLS / security
	Security       string   `json:"security,omitempty"`
	SNI            string   `json:"sni,omitempty"`
	ALPN           []string `json:"alpn,omitempty"`
	Fingerprint    string   `json:"fingerprint,omitempty"`
	SkipCertVerify bool     `json:"skipCertVerify,omitempty"`
	RealityPublic  string   `json:"realityPublicKey,omitempty"`
	RealityShortID string   `json:"realityShortId,omitempty"`

	// Transport
	Network     string            `json:"network,omitempty"`
	Path        string            `json:"path,omitempty"`
	Host        string            `json:"host,omitempty"`
	ServiceName string            `json:"serviceName,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`

	// SS plugin
	Plugin     string            `json:"plugin,omitempty"`
	PluginOpts map[string]string `json:"pluginOpts,omitempty"`

	// Hysteria2 / TUIC extras
	Obfs         string `json:"obfs,omitempty"`
	ObfsPassword string `json:"obfsPassword,omitempty"`
	Congestion   string `json:"congestion,omitempty"`
	UDPRelayMode string `json:"udpRelayMode,omitempty"`

	// Extra preserves protocol fields that have no dedicated field yet.
	Extra map[string]any `json:"extra,omitempty"`
}

// Validate reports whether the node has the minimum fields required to be
// emitted for any target platform.
func (n Node) Validate() error {
	if strings.TrimSpace(n.Name) == "" {
		return fmt.Errorf("node name is required")
	}
	if strings.TrimSpace(n.Server) == "" {
		return fmt.Errorf("node %q has no server", n.Name)
	}
	if n.Port <= 0 || n.Port > 65535 {
		return fmt.Errorf("node %q has invalid port %d", n.Name, n.Port)
	}
	switch n.Type {
	case TypeVLESS, TypeVMess, TypeTUIC:
		if n.UUID == "" {
			return fmt.Errorf("node %q (%s) requires a uuid", n.Name, n.Type)
		}
	case TypeTrojan, TypeHysteria2:
		if n.Password == "" {
			return fmt.Errorf("node %q (%s) requires a password", n.Name, n.Type)
		}
	case TypeSS:
		if n.Cipher == "" || n.Password == "" {
			return fmt.Errorf("node %q (shadowsocks) requires cipher and password", n.Name)
		}
	default:
		return fmt.Errorf("node %q has unsupported type %q", n.Name, n.Type)
	}
	return nil
}

// HasTLS reports whether the node uses TLS or REALITY.
func (n Node) HasTLS() bool {
	return n.Security == SecurityTLS || n.Security == SecurityReality || n.Type == TypeTrojan ||
		n.Type == TypeHysteria2 || n.Type == TypeTUIC
}

// EffectiveSNI returns the SNI, falling back to the server address.
func (n Node) EffectiveSNI() string {
	if n.SNI != "" {
		return n.SNI
	}
	if n.Host != "" {
		return n.Host
	}
	return n.Server
}

// serverPort formats the address for line-based producers.
func (n Node) serverPort() string {
	if strings.Contains(n.Server, ":") {
		// IPv6 literal
		return "[" + n.Server + "]:" + strconv.Itoa(n.Port)
	}
	return n.Server + ":" + strconv.Itoa(n.Port)
}

func (n Node) clone() Node {
	out := n
	if n.ALPN != nil {
		out.ALPN = append([]string(nil), n.ALPN...)
	}
	if n.Headers != nil {
		out.Headers = make(map[string]string, len(n.Headers))
		for k, v := range n.Headers {
			out.Headers[k] = v
		}
	}
	if n.PluginOpts != nil {
		out.PluginOpts = make(map[string]string, len(n.PluginOpts))
		for k, v := range n.PluginOpts {
			out.PluginOpts[k] = v
		}
	}
	if n.Extra != nil {
		out.Extra = make(map[string]any, len(n.Extra))
		for k, v := range n.Extra {
			out.Extra[k] = v
		}
	}
	return out
}

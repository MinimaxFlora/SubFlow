package proxy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ParseSubscription parses the body of a subscription into nodes. It accepts
// a plain URI list, a Base64-encoded URI list, or a JSON array of nodes.
func ParseSubscription(body string) ([]Node, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return nil, nil
	}

	// JSON array of nodes (V2Ray-style or unified model).
	if strings.HasPrefix(trimmed, "[") {
		if nodes, err := parseJSONNodes(trimmed); err == nil {
			return nodes, nil
		}
	}

	// Base64-wrapped URI list.
	if decoded, ok := tryBase64(trimmed); ok {
		return parseURIList(decoded)
	}

	return parseURIList(trimmed)
}

func parseJSONNodes(body string) ([]Node, error) {
	var raw []map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(raw))
	for _, item := range raw {
		node, err := parseJSONNode(item)
		if err != nil {
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func parseJSONNode(raw map[string]any) (Node, error) {
	node := Node{
		Name:   str(raw["name"]),
		Server: str(raw["server"]),
	}
	if node.Name == "" {
		node.Name = str(raw["ps"])
	}
	node.Port = intOf(raw["port"])
	node.Type = normalizeType(str(raw["type"]))
	node.UUID = firstNonEmpty(str(raw["uuid"]), str(raw["id"]), str(raw["password"]))
	node.Password = firstNonEmpty(str(raw["password"]), str(raw["id"]))
	node.AlterID = intOf(raw["alterId"])
	node.Cipher = str(raw["cipher"])
	node.Flow = str(raw["flow"])
	if err := node.Validate(); err != nil {
		return Node{}, err
	}
	return node, nil
}

func parseURIList(body string) ([]Node, error) {
	lines := strings.Split(body, "\n")
	nodes := make([]Node, 0, len(lines))
	var failures []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		node, err := ParseURI(line)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 0 && len(failures) > 0 {
		return nil, fmt.Errorf("no nodes parsed: %s", failures[0])
	}
	return nodes, nil
}

// ParseURI parses a single share link into a Node.
func ParseURI(link string) (Node, error) {
	scheme := link
	if idx := strings.Index(link, "://"); idx >= 0 {
		scheme = strings.ToLower(link[:idx])
	}
	switch scheme {
	case "vless":
		return parseVLESS(link)
	case "trojan", "trojan-go":
		return parseTrojan(link)
	case "ss":
		return parseSS(link)
	case "vmess":
		return parseVMess(link)
	case "hysteria2", "hy2":
		return parseHysteria2(link)
	case "tuic":
		return parseTUIC(link)
	default:
		return Node{}, fmt.Errorf("unsupported scheme %q", scheme)
	}
}

func parseVLESS(link string) (Node, error) {
	u, err := url.Parse(link)
	if err != nil {
		return Node{}, fmt.Errorf("vless: %w", err)
	}
	node := Node{
		Type:   TypeVLESS,
		UUID:   u.User.Username(),
		Server: u.Hostname(),
		Name:   fragmentName(u),
	}
	node.Port, err = portOf(u)
	if err != nil {
		return Node{}, err
	}
	q := u.Query()
	node.Security = securityOf(q)
	node.SNI = q.Get("sni")
	node.Fingerprint = q.Get("fp")
	node.ALPN = splitList(q.Get("alpn"))
	node.SkipCertVerify = truthy(q.Get("allowInsecure")) || truthy(q.Get("insecure"))
	node.RealityPublic = firstNonEmpty(q.Get("pbk"), q.Get("publicKey"))
	node.RealityShortID = firstNonEmpty(q.Get("sid"), q.Get("shortId"))
	node.Flow = q.Get("flow")
	applyTransport(&node, q)
	return node, node.Validate()
}

func parseTrojan(link string) (Node, error) {
	u, err := url.Parse(link)
	if err != nil {
		return Node{}, fmt.Errorf("trojan: %w", err)
	}
	node := Node{
		Type:     TypeTrojan,
		Password: u.User.Username(),
		Server:   u.Hostname(),
		Name:     fragmentName(u),
		Security: SecurityTLS,
	}
	node.Port, err = portOf(u)
	if err != nil {
		return Node{}, err
	}
	q := u.Query()
	// Trojan defaults to TLS unless explicitly disabled.
	if strings.EqualFold(q.Get("security"), "none") {
		node.Security = SecurityNone
	}
	node.SNI = q.Get("sni")
	node.ALPN = splitList(q.Get("alpn"))
	node.Fingerprint = q.Get("fp")
	node.SkipCertVerify = truthy(q.Get("allowInsecure")) || truthy(q.Get("insecure"))
	applyTransport(&node, q)
	return node, node.Validate()
}

func parseSS(link string) (Node, error) {
	rest := strings.TrimPrefix(link, "ss://")
	name := ""
	if idx := strings.Index(rest, "#"); idx >= 0 {
		if decoded, err := url.QueryUnescape(rest[idx+1:]); err == nil {
			name = decoded
		}
		rest = rest[:idx]
	}
	// strip query (plugin)
	query := ""
	if idx := strings.Index(rest, "?"); idx >= 0 {
		query = rest[idx+1:]
		rest = rest[:idx]
	}

	var userinfo string
	serverPort := ""
	if strings.Contains(rest, "@") {
		parts := strings.SplitN(rest, "@", 2)
		userinfo = parts[0]
		serverPort = parts[1]
		if decoded, ok := decodeBase64(userinfo); ok {
			userinfo = decoded
		}
	} else {
		decoded, ok := decodeBase64(rest)
		if !ok {
			return Node{}, fmt.Errorf("ss: invalid encoding")
		}
		if strings.Contains(decoded, "@") {
			parts := strings.SplitN(decoded, "@", 2)
			userinfo = parts[0]
			serverPort = parts[1]
		} else {
			return Node{}, fmt.Errorf("ss: missing server")
		}
	}

	node := Node{Type: TypeSS, Name: name, Security: SecurityNone}
	methodPass := strings.SplitN(userinfo, ":", 2)
	if len(methodPass) != 2 {
		return Node{}, fmt.Errorf("ss: invalid method:password")
	}
	node.Cipher = strings.ToLower(methodPass[0])
	node.Password = methodPass[1]

	host, portStr, err := net.SplitHostPort(serverPort)
	if err != nil {
		return Node{}, fmt.Errorf("ss: invalid address %q", serverPort)
	}
	node.Server = host
	node.Port, err = strconv.Atoi(portStr)
	if err != nil {
		return Node{}, fmt.Errorf("ss: invalid port")
	}
	if query != "" {
		q, _ := url.ParseQuery(query)
		node.Plugin = q.Get("plugin")
	}
	return node, node.Validate()
}

func parseVMess(link string) (Node, error) {
	payload := strings.TrimPrefix(link, "vmess://")
	decoded, ok := decodeBase64(payload)
	if !ok {
		return Node{}, fmt.Errorf("vmess: invalid base64 payload")
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(decoded), &raw); err != nil {
		return Node{}, fmt.Errorf("vmess: invalid json payload")
	}
	node := Node{
		Type:     TypeVMess,
		Name:     str(raw["ps"]),
		Server:   str(raw["add"]),
		UUID:     str(raw["id"]),
		AlterID:  intOf(raw["aid"]),
		Security: SecurityNone,
	}
	node.Port = intOf(raw["port"])
	if scy := str(raw["scy"]); scy != "" && scy != "auto" {
		node.Cipher = scy
	}
	if str(raw["tls"]) == "tls" {
		node.Security = SecurityTLS
	}
	node.SNI = str(raw["sni"])
	node.ALPN = splitList(str(raw["alpn"]))
	node.Fingerprint = str(raw["fp"])
	net := str(raw["net"])
	node.Network = net
	switch net {
	case NetworkWS:
		node.Path = str(raw["path"])
		node.Host = str(raw["host"])
	case NetworkGRPC:
		node.ServiceName = str(raw["path"])
		node.Host = str(raw["host"])
	case NetworkH2, NetworkHTTP:
		node.Path = str(raw["path"])
		node.Host = str(raw["host"])
	}
	return node, node.Validate()
}

func parseHysteria2(link string) (Node, error) {
	u, err := url.Parse(link)
	if err != nil {
		return Node{}, fmt.Errorf("hysteria2: %w", err)
	}
	node := Node{
		Type:     TypeHysteria2,
		Password: firstNonEmpty(u.User.Username(), mustUserPassword(u)),
		Server:   u.Hostname(),
		Name:     fragmentName(u),
		Security: SecurityTLS,
	}
	node.Port, err = portOf(u)
	if err != nil {
		return Node{}, err
	}
	q := u.Query()
	node.SNI = q.Get("sni")
	node.SkipCertVerify = truthy(q.Get("insecure"))
	node.Obfs = q.Get("obfs")
	node.ObfsPassword = q.Get("obfs-password")
	node.ALPN = splitList(q.Get("alpn"))
	return node, node.Validate()
}

func parseTUIC(link string) (Node, error) {
	u, err := url.Parse(link)
	if err != nil {
		return Node{}, fmt.Errorf("tuic: %w", err)
	}
	node := Node{
		Type:     TypeTUIC,
		Server:   u.Hostname(),
		Name:     fragmentName(u),
		Security: SecurityTLS,
	}
	node.Port, err = portOf(u)
	if err != nil {
		return Node{}, err
	}
	node.UUID = u.User.Username()
	node.Password = mustUserPassword(u)
	q := u.Query()
	node.SNI = q.Get("sni")
	node.ALPN = splitList(q.Get("alpn"))
	node.SkipCertVerify = truthy(q.Get("allow_insecure")) || truthy(q.Get("insecure"))
	node.Congestion = q.Get("congestion_control")
	node.UDPRelayMode = q.Get("udp_relay_mode")
	return node, node.Validate()
}

func applyTransport(node *Node, q url.Values) {
	network := strings.ToLower(q.Get("type"))
	if network == "" {
		network = NetworkTCP
	}
	node.Network = network
	switch network {
	case NetworkWS:
		node.Path = firstNonEmpty(q.Get("path"), "/")
		node.Host = q.Get("host")
	case NetworkGRPC:
		node.ServiceName = q.Get("serviceName")
		node.Host = q.Get("host")
	case NetworkH2, NetworkHTTP:
		node.Path = firstNonEmpty(q.Get("path"), "/")
		node.Host = q.Get("host")
	}
}

func securityOf(q url.Values) string {
	switch s := strings.ToLower(q.Get("security")); s {
	case "tls":
		return SecurityTLS
	case "reality":
		return SecurityReality
	case "none", "":
		return SecurityNone
	default:
		return s
	}
}

func fragmentName(u *url.URL) string {
	if u.Fragment == "" {
		return u.Hostname()
	}
	if decoded, err := url.QueryUnescape(u.Fragment); err == nil {
		return decoded
	}
	return u.Fragment
}

func portOf(u *url.URL) (int, error) {
	if u.Port() == "" {
		return 0, fmt.Errorf("missing port in %s", u.Host)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", u.Port())
	}
	return p, nil
}

func mustUserPassword(u *url.URL) string {
	if u.User == nil {
		return ""
	}
	pw, _ := u.User.Password()
	return pw
}

func normalizeType(t string) string {
	switch strings.ToLower(t) {
	case TypeVLESS:
		return TypeVLESS
	case TypeVMess:
		return TypeVMess
	case TypeTrojan:
		return TypeTrojan
	case TypeSS, "ss":
		return TypeSS
	case TypeHysteria2:
		return TypeHysteria2
	case TypeTUIC:
		return TypeTUIC
	default:
		return strings.ToLower(t)
	}
}

func tryBase64(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	candidates := []string{value, padBase64(value)}
	for _, c := range candidates {
		for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
			if decoded, err := enc.DecodeString(c); err == nil {
				text := string(decoded)
				if strings.Contains(text, "://") || strings.Contains(text, "\n") {
					return text, true
				}
			}
		}
	}
	return "", false
}

// decodeBase64 decodes a base64 string using the common alphabets without
// imposing any content heuristic on the result.
func decodeBase64(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	for _, c := range []string{value, padBase64(value)} {
		for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
			if decoded, err := enc.DecodeString(c); err == nil {
				return string(decoded), true
			}
		}
	}
	return "", false
}

func padBase64(value string) string {
	if m := len(value) % 4; m != 0 {
		return value + strings.Repeat("=", 4-m)
	}
	return value
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '|' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func intOf(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

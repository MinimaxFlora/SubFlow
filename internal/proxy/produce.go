package proxy

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
)

// Platform identifies a target client format.
type Platform struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Extension string `json:"extension"`
	Group     string `json:"group"`
}

// Platforms lists every supported output target.
var Platforms = []Platform{
	{ID: "uri", Name: "URI", Extension: "txt", Group: "generic"},
	{ID: "base64", Name: "Base64", Extension: "txt", Group: "generic"},
	{ID: "clash", Name: "Clash", Extension: "yaml", Group: "clash"},
	{ID: "clashmeta", Name: "Clash Meta", Extension: "yaml", Group: "clash"},
	{ID: "stash", Name: "Stash", Extension: "yaml", Group: "clash"},
	{ID: "singbox", Name: "sing-box", Extension: "json", Group: "json"},
	{ID: "v2ray", Name: "V2Ray", Extension: "json", Group: "json"},
	{ID: "surge", Name: "Surge", Extension: "conf", Group: "line"},
	{ID: "surgemac", Name: "Surge Mac", Extension: "conf", Group: "line"},
	{ID: "loon", Name: "Loon", Extension: "conf", Group: "line"},
	{ID: "qx", Name: "Quantumult X", Extension: "conf", Group: "line"},
	{ID: "surfboard", Name: "Surfboard", Extension: "conf", Group: "line"},
	{ID: "shadowrocket", Name: "Shadowrocket", Extension: "conf", Group: "line"},
}

// IsPlatform reports whether id is a known target.
func IsPlatform(id string) bool {
	for _, p := range Platforms {
		if p.ID == id {
			return true
		}
	}
	return false
}

// Options tunes production behaviour.
type Options struct {
	// IncludeUnsupported keeps nodes a target cannot represent, emitting the
	// closest equivalent instead of dropping them.
	IncludeUnsupported bool
}

// Produce renders nodes into the requested target platform.
func Produce(nodes []Node, platform string, opts Options) (string, error) {
	nodes = filterForPlatform(nodes, platform, opts)
	switch platform {
	case "uri", "shadowrocket":
		return produceURIList(nodes)
	case "base64":
		list, err := produceURIList(nodes)
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString([]byte(list)), nil
	case "clash", "clashmeta", "stash":
		return produceClash(nodes, platform, opts)
	case "singbox":
		return produceSingBox(nodes, opts)
	case "v2ray":
		return produceV2Ray(nodes, opts)
	case "surge", "surgemac":
		return produceSurge(nodes), nil
	case "loon":
		return produceLoon(nodes), nil
	case "qx":
		return produceQX(nodes), nil
	case "surfboard":
		return produceSurfboard(nodes), nil
	default:
		return "", fmt.Errorf("unsupported platform %q", platform)
	}
}

func produceURIList(nodes []Node) (string, error) {
	lines := make([]string, 0, len(nodes))
	for _, node := range nodes {
		uri, err := node.ToURI()
		if err != nil {
			continue
		}
		lines = append(lines, uri)
	}
	return strings.Join(lines, "\n"), nil
}

// Supports reports whether a target platform can represent the node.
func Supports(platform string, n Node) bool {
	switch platform {
	case "uri", "base64", "shadowrocket", "singbox":
		return true
	case "v2ray":
		switch n.Type {
		case TypeVLESS, TypeVMess, TypeTrojan, TypeSS:
			return true
		default:
			return false
		}
	case "clash", "clashmeta", "stash":
		switch n.Type {
		case TypeVLESS, TypeVMess, TypeTrojan, TypeSS:
			// The original Clash cannot express VLESS + REALITY, but
			// Clash.Meta (mihomo) and Stash can.
			if n.Type == TypeVLESS && n.Security == SecurityReality {
				return platform == "clashmeta" || platform == "stash"
			}
			return true
		case TypeHysteria2, TypeTUIC:
			return platform == "clashmeta" || platform == "stash"
		default:
			return false
		}
	case "surge", "surgemac", "loon":
		switch n.Type {
		case TypeSS, TypeVMess, TypeTrojan, TypeVLESS, TypeHysteria2, TypeTUIC:
			return true
		default:
			return false
		}
	case "qx":
		switch n.Type {
		case TypeSS, TypeVMess, TypeTrojan, TypeVLESS, TypeHysteria2:
			return true
		default:
			return false
		}
	case "surfboard":
		switch n.Type {
		case TypeSS, TypeVMess, TypeTrojan, TypeVLESS:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// filterForPlatform keeps only nodes representable by the target unless the
// caller explicitly opts into keeping unsupported nodes.
func filterForPlatform(nodes []Node, platform string, opts Options) []Node {
	if opts.IncludeUnsupported {
		return nodes
	}
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if Supports(platform, n) {
			out = append(out, n)
		}
	}
	return out
}

// Registry returns platforms sorted for stable UI output.
func Registry() []Platform {
	out := append([]Platform(nil), Platforms...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

package proxy

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Operation is a single node-processing step.
type Operation struct {
	Type string         `json:"type"`
	Args map[string]any `json:"args"`
}

// ApplyOperations runs the ordered operation chain over the nodes.
func ApplyOperations(nodes []Node, ops []Operation) ([]Node, error) {
	current := make([]Node, len(nodes))
	for i, n := range nodes {
		current[i] = n.clone()
	}
	for idx, op := range ops {
		out, err := applyOperation(current, op)
		if err != nil {
			return nil, fmt.Errorf("operation %d (%s): %w", idx, op.Type, err)
		}
		current = out
	}
	return current, nil
}

func applyOperation(nodes []Node, op Operation) ([]Node, error) {
	switch strings.ToLower(op.Type) {
	case "", "noop":
		return nodes, nil
	case "filter", "regexfilter":
		return opFilter(nodes, op.Args)
	case "rename", "regexrename":
		return opRename(nodes, op.Args)
	case "deduplicate", "dedupe":
		return opDeduplicate(nodes, op.Args), nil
	case "sort":
		return opSort(nodes, op.Args)
	case "uselessfilter":
		return opUselessFilter(nodes), nil
	case "setproperty":
		return opSetProperty(nodes, op.Args)
	default:
		return nil, fmt.Errorf("unknown operation type %q", op.Type)
	}
}

func opFilter(nodes []Node, args map[string]any) ([]Node, error) {
	pattern := argString(args, "regex", "pattern")
	if pattern == "" {
		return nil, fmt.Errorf("filter requires a regex")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %w", err)
	}
	mode := strings.ToLower(argString(args, "mode"))
	field := strings.ToLower(argString(args, "field"))
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		target := filterField(n, field)
		matched := re.MatchString(target)
		keep := matched
		if mode == "exclude" {
			keep = !matched
		}
		if keep {
			out = append(out, n)
		}
	}
	return out, nil
}

func filterField(n Node, field string) string {
	switch field {
	case "", "name":
		return n.Name
	case "server", "host":
		return n.Server
	case "type", "protocol":
		return n.Type
	default:
		return n.Name
	}
}

func opRename(nodes []Node, args map[string]any) ([]Node, error) {
	out := make([]Node, len(nodes))
	copy(out, nodes)

	prefix := argString(args, "prefix")
	suffix := argString(args, "suffix")
	if prefix != "" || suffix != "" {
		for i := range out {
			out[i].Name = prefix + out[i].Name + suffix
		}
		return out, nil
	}

	pattern := argString(args, "regex", "pattern")
	if pattern == "" {
		return out, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %w", err)
	}
	replacement := argString(args, "replacement")
	for i := range out {
		out[i].Name = re.ReplaceAllString(out[i].Name, replacement)
	}
	return out, nil
}

func opDeduplicate(nodes []Node, args map[string]any) []Node {
	by := strings.ToLower(argString(args, "by"))
	seen := make(map[string]struct{}, len(nodes))
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		var key string
		switch by {
		case "server", "address":
			key = fmt.Sprintf("%s:%d", n.Server, n.Port)
		case "full", "identity":
			key = fmt.Sprintf("%s|%s|%s:%d", n.Type, n.UUID+n.Password, n.Server, n.Port)
		default:
			key = n.Name
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, n)
	}
	return out
}

func opSort(nodes []Node, args map[string]any) ([]Node, error) {
	by := strings.ToLower(argString(args, "by"))
	desc := strings.EqualFold(argString(args, "order"), "desc")
	sort.SliceStable(nodes, func(i, j int) bool {
		var less bool
		switch by {
		case "server", "address":
			less = nodes[i].Server < nodes[j].Server
		case "type", "protocol":
			less = nodes[i].Type < nodes[j].Type
		case "port":
			less = nodes[i].Port < nodes[j].Port
		default:
			less = nodes[i].Name < nodes[j].Name
		}
		if desc {
			return !less
		}
		return less
	})
	return nodes, nil
}

// opUselessFilter removes nodes missing the fields required to connect.
func opUselessFilter(nodes []Node) []Node {
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if err := n.Validate(); err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(n.Name), "expire") ||
			strings.Contains(strings.ToLower(n.Name), "剩余流量") {
			continue
		}
		out = append(out, n)
	}
	return out
}

func opSetProperty(nodes []Node, args map[string]any) ([]Node, error) {
	key := strings.ToLower(argString(args, "key"))
	value := argString(args, "value")
	if key == "" {
		return nil, fmt.Errorf("setproperty requires a key")
	}
	for i := range nodes {
		switch key {
		case "name":
			nodes[i].Name = value
		case "sni":
			nodes[i].SNI = value
		case "host":
			nodes[i].Host = value
		case "path":
			nodes[i].Path = value
		default:
			return nil, fmt.Errorf("setproperty does not support key %q", key)
		}
	}
	return nodes, nil
}

func argString(args map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := args[key]; ok {
			if s := str(v); s != "" {
				return s
			}
		}
	}
	return ""
}

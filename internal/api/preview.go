package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/subflow-org/subflow/internal/proxy"
	"github.com/subflow-org/subflow/internal/storage"
)

// processStep is one entry of a subscription's `process` chain. The frontend
// stores a rich, evolving set of processor types; unknown ones are reported and
// skipped rather than failing the whole request.
type processStep struct {
	Type     string         `json:"type"`
	Args     map[string]any `json:"args"`
	Disabled bool           `json:"disabled"`
}

type subDocument struct {
	Name        string        `json:"name"`
	DisplayName string        `json:"displayName"`
	Source      string        `json:"source"`
	URL         string        `json:"url"`
	Content     string        `json:"content"`
	UA          string        `json:"ua"`
	Process     []processStep `json:"process"`
}

type collectionDocument struct {
	Name          string        `json:"name"`
	DisplayName   string        `json:"displayName"`
	Subscriptions []string      `json:"subscriptions"`
	Process       []processStep `json:"process"`
}

type fileDocument struct {
	Name    string        `json:"name"`
	Content string        `json:"content"`
	Process []processStep `json:"process"`
}

// handlePreview parses and processes a subscription without persisting it, and
// returns the parsed and processed nodes for the comparison table.
//
//	POST /api/preview/{type}
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request, typeParam string) {
	var raw json.RawMessage
	if !restDecode(w, r, &raw) {
		return
	}

	original, process, err := s.resolveDocument(r.Context(), typeParam, raw, map[string]bool{})
	if err != nil {
		restBadRequest(w, "PREVIEW_FAILED", err.Error())
		return
	}

	processed := original
	if len(process) > 0 {
		ops, skipped := toOperations(process)
		s.logSkippedProcessors(typeParam, skipped)
		if len(ops) > 0 {
			processed, err = proxy.ApplyOperations(original, ops)
			if err != nil {
				restBadRequest(w, "PREVIEW_FAILED", err.Error())
				return
			}
		}
	}

	restOK(w, map[string]any{
		"original":  toPreviewNodes(original),
		"processed": toPreviewNodes(processed),
	})
}

// handleDownload renders a stored subscription, collection or file into the
// requested target platform. This is the URL users give to their clients.
//
//	GET /download/{name} and GET /download/collection/{name}
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request, kind, name string) {
	doc, err := s.deps.Resources.Get(r.Context(), kind, name)
	if err == storage.ErrResourceNotFound {
		restNotFound(w, "资源不存在")
		return
	}
	if err != nil {
		s.logError("load download source", err, "kind", kind, "name", name)
		restInternal(w, "读取数据失败")
		return
	}

	query := r.URL.Query()
	target := query.Get("target")
	if target == "" {
		target = "base64"
	}
	if !proxy.IsPlatform(target) {
		restBadRequest(w, "UNKNOWN_PLATFORM", "不支持的目标平台: "+target)
		return
	}

	nodes, process, err := s.resolveDocument(r.Context(), kind, json.RawMessage(doc.Data), map[string]bool{})
	if err != nil {
		restBadRequest(w, "CONVERT_FAILED", err.Error())
		return
	}

	if len(process) > 0 {
		ops, skipped := toOperations(process)
		s.logSkippedProcessors(kind, skipped)
		if len(ops) > 0 {
			nodes, err = proxy.ApplyOperations(nodes, ops)
			if err != nil {
				restBadRequest(w, "CONVERT_FAILED", err.Error())
				return
			}
		}
	}

	includeUnsupported := query.Get("includeUnsupportedProxy") != ""
	output, err := proxy.Produce(nodes, target, proxy.Options{IncludeUnsupported: includeUnsupported})
	if err != nil {
		restBadRequest(w, "CONVERT_FAILED", err.Error())
		return
	}

	contentType := "text/plain; charset=utf-8"
	if ext := platformExtension(target); ext == "yaml" || ext == "json" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", name+"."+platformExtension(target)))
	_, _ = w.Write([]byte(output))
}

// resolveDocument turns a stored document into parsed proxy nodes plus the
// processor chain declared on it.
func (s *Server) resolveDocument(
	ctx context.Context,
	kind string,
	raw json.RawMessage,
	seen map[string]bool,
) ([]proxy.Node, []processStep, error) {
	switch kind {
	case "sub", "subs":
		var doc subDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, nil, fmt.Errorf("订阅数据格式不正确")
		}
		content, err := s.subscriptionContent(ctx, doc)
		if err != nil {
			return nil, nil, err
		}
		nodes, err := proxy.ParseSubscription(content)
		if err != nil {
			return nil, nil, err
		}
		return nodes, doc.Process, nil

	case "collection", "collections":
		var doc collectionDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, nil, fmt.Errorf("组合订阅数据格式不正确")
		}
		var nodes []proxy.Node
		for _, member := range doc.Subscriptions {
			if seen[member] {
				return nil, nil, fmt.Errorf("组合订阅存在循环引用: %s", member)
			}
			seen[member] = true
			memberDoc, err := s.deps.Resources.Get(ctx, storage.KindSub, member)
			if err != nil {
				return nil, nil, fmt.Errorf("组合订阅引用的 %s 不存在", member)
			}
			memberNodes, _, err := s.resolveDocument(ctx, "sub", json.RawMessage(memberDoc.Data), seen)
			if err != nil {
				return nil, nil, err
			}
			nodes = append(nodes, memberNodes...)
		}
		return nodes, doc.Process, nil

	case "file", "files":
		var doc fileDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, nil, fmt.Errorf("文件数据格式不正确")
		}
		nodes, err := proxy.ParseSubscription(doc.Content)
		if err != nil {
			return nil, nil, err
		}
		return nodes, doc.Process, nil

	default:
		return nil, nil, fmt.Errorf("不支持的资源类型: %s", kind)
	}
}

// subscriptionContent returns the subscription body, fetching it when the
// subscription is remote.
func (s *Server) subscriptionContent(ctx context.Context, doc subDocument) (string, error) {
	if doc.Source == "local" || doc.URL == "" {
		return doc.Content, nil
	}
	if s.deps.Fetcher == nil {
		return "", fmt.Errorf("服务器未配置抓取器，无法获取远程订阅")
	}
	result, err := s.deps.Fetcher.Get(ctx, doc.URL, doc.UA)
	if err != nil {
		return "", fmt.Errorf("获取订阅失败: %w", err)
	}
	return result.Content, nil
}

// toOperations maps the frontend processor chain onto pipeline operations.
// Unsupported processor types are returned so the caller can report them.
func toOperations(steps []processStep) ([]proxy.Operation, []string) {
	ops := make([]proxy.Operation, 0, len(steps))
	var skipped []string

	for _, step := range steps {
		if step.Disabled {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(step.Type))
		name = strings.TrimSuffix(name, "processor")
		args := step.Args
		if args == nil {
			args = map[string]any{}
		}

		switch name {
		case "filter", "regexfilter", "regexfilterproxy":
			regex := firstString(args, "regex", "pattern", "keywords")
			if regex == "" {
				continue
			}
			ops = append(ops, proxy.Operation{Type: "filter", Args: map[string]any{
				"regex": regex,
				"mode":  modeFromArgs(args),
			}})
		case "rename", "regexrename":
			pattern := firstString(args, "pattern", "regex")
			if pattern == "" {
				continue
			}
			ops = append(ops, proxy.Operation{Type: "rename", Args: map[string]any{
				"regex":       pattern,
				"replacement": firstString(args, "replace", "replacement"),
			}})
		case "deduplicate", "dedup":
			ops = append(ops, proxy.Operation{Type: "deduplicate", Args: map[string]any{
				"by": firstString(args, "by", "field"),
			}})
		case "sort":
			ops = append(ops, proxy.Operation{Type: "sort", Args: map[string]any{
				"by":    firstString(args, "sortBy", "by"),
				"order": firstString(args, "order"),
			}})
		case "uselessfilter", "useless":
			ops = append(ops, proxy.Operation{Type: "uselessfilter", Args: map[string]any{}})
		case "setproperty":
			ops = append(ops, proxy.Operation{Type: "setproperty", Args: map[string]any{
				"key":   firstString(args, "key"),
				"value": firstString(args, "value"),
			}})
		default:
			skipped = append(skipped, step.Type)
		}
	}
	return ops, skipped
}

// modeFromArgs derives include/exclude from the many spellings the frontend
// uses for a filter direction.
func modeFromArgs(args map[string]any) string {
	if strings.EqualFold(firstString(args, "mode"), "exclude") {
		return "exclude"
	}
	if value, ok := args["exclude"]; ok {
		if flag, ok := value.(bool); ok && flag {
			return "exclude"
		}
	}
	return "include"
}

// firstString reads the first present key, accepting string values, a single
// element array, or a scalar.
func firstString(args map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := args[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if typed != "" {
				return typed
			}
		case []any:
			if len(typed) > 0 {
				if s, ok := typed[0].(string); ok && s != "" {
					return s
				}
			}
		case float64:
			return fmt.Sprintf("%v", typed)
		}
	}
	return ""
}

// toPreviewNodes renders nodes for the comparison table using the kebab-case
// field names the frontend expects.
func toPreviewNodes(nodes []proxy.Node) []map[string]any {
	out := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		item := map[string]any{
			"id":     nodeIdentity(node),
			"name":   node.Name,
			"type":   node.Type,
			"server": node.Server,
			"port":   node.Port,
			"udp":    true,
			"tfo":    false,
		}
		if node.SkipCertVerify {
			item["skip-cert-verify"] = true
		}
		if node.Extra != nil {
			for key, value := range node.Extra {
				if _, exists := item[key]; !exists {
					item[key] = value
				}
			}
		}
		out = append(out, item)
	}
	return out
}

// nodeIdentity produces a stable id so the comparison table can diff the
// original and processed lists.
func nodeIdentity(node proxy.Node) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%d|%s|%s",
		node.Type, node.Server, node.UUID, node.Port, node.Password, node.Network)))
	return hex.EncodeToString(sum[:8])
}

func platformExtension(target string) string {
	for _, p := range proxy.Platforms {
		if p.ID == target {
			return p.Extension
		}
	}
	return "txt"
}

// logSkippedProcessors records processor types that have no pipeline
// equivalent yet, so gaps are visible in the server log.
func (s *Server) logSkippedProcessors(kind string, skipped []string) {
	if len(skipped) == 0 {
		return
	}
	slog.Warn("skipped unsupported processors", "kind", kind, "types", strings.Join(skipped, ","))
}

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
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
	URL     string        `json:"url"`
	Source  string        `json:"source"`
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

	original, process, err := s.resolveDocument(r.Context(), typeParam, raw, map[string]bool{}, false)
	if err != nil {
		restInternalError(w, "INTERNAL_SERVER_ERROR", "Failed to preview "+previewSubject(typeParam), err.Error())
		return
	}

	processed := original
	if len(process) > 0 {
		ops, skipped := toOperations(process)
		s.logSkippedProcessors(typeParam, skipped)
		if len(ops) > 0 {
			processed, err = proxy.ApplyOperations(original, ops)
			if err != nil {
				restInternalError(w, "INTERNAL_SERVER_ERROR", "Failed to preview "+previewSubject(typeParam), err.Error())
				return
			}
		}
	}

	restSuccess(w, map[string]any{
		"original":  toPreviewNodes(original),
		"processed": toPreviewNodes(processed),
	})
}

// previewSubject names the document a preview request targets, matching the
// wording of Sub-Store's preview error messages.
func previewSubject(typeParam string) string {
	switch typeParam {
	case "collection", "collections":
		return "collection"
	case "file", "files":
		return "file"
	default:
		return "subscription"
	}
}

// handleDownload renders a stored subscription, collection or file into the
// requested target platform. This is the URL users give to their clients.
//
//	GET /download/{name} and GET /download/collection/{name}
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request, kind, name string) {
	doc, err := s.deps.Resources.Get(r.Context(), kind, name)
	if err == storage.ErrResourceNotFound {
		restNotFound(w, "RESOURCE_NOT_FOUND", fmt.Sprintf("%s %s does not exist!", resourceLabel(kind), name))
		return
	}
	if err != nil {
		s.logError("load download source", err, "kind", kind, "name", name)
		restInternalError(w, "INTERNAL_SERVER_ERROR", downloadFailure(kind, name), err.Error())
		return
	}

	query := r.URL.Query()
	target, ok := resolveTarget(query, r.PathValue("target"), r.Header)
	if !ok {
		restInternalError(w, "INTERNAL_SERVER_ERROR", downloadFailure(kind, name), "unsupported target platform: "+requestedTarget(query, r.PathValue("target"), r.Header))
		return
	}

	noCache := query.Get("noCache") != ""
	nodes, process, err := s.resolveDocument(r.Context(), kind, json.RawMessage(doc.Data), map[string]bool{}, noCache)
	if err != nil {
		restInternalError(w, "INTERNAL_SERVER_ERROR", downloadFailure(kind, name), err.Error())
		return
	}

	if len(process) > 0 {
		ops, skipped := toOperations(process)
		s.logSkippedProcessors(kind, skipped)
		if len(ops) > 0 {
			nodes, err = proxy.ApplyOperations(nodes, ops)
			if err != nil {
				restInternalError(w, "INTERNAL_SERVER_ERROR", downloadFailure(kind, name), err.Error())
				return
			}
		}
	}

	includeUnsupported := query.Get("includeUnsupportedProxy") != ""
	output, err := proxy.Produce(nodes, target, proxy.Options{IncludeUnsupported: includeUnsupported})
	if err != nil {
		restInternalError(w, "INTERNAL_SERVER_ERROR", downloadFailure(kind, name), err.Error())
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

// requestedTarget returns the raw target the client asked for, so error
// messages can echo it back.
func requestedTarget(query url.Values, pathTarget string, header http.Header) string {
	if v := query.Get("platform"); v != "" {
		return v
	}
	if pathTarget != "" {
		return pathTarget
	}
	if v := query.Get("target"); v != "" {
		return v
	}
	if v := platformFromUserAgent(header); v != "" {
		return v
	}
	return "JSON"
}

// resolveTarget normalizes the requested output platform, mirroring Sub-Store:
// the `platform` query parameter, then the `target` path or query parameter,
// then User-Agent detection, and finally a JSON fallback.
func resolveTarget(query url.Values, pathTarget string, header http.Header) (string, bool) {
	for _, raw := range []string{query.Get("platform"), pathTarget, query.Get("target")} {
		if raw == "" {
			continue
		}
		return proxy.CanonicalPlatform(raw)
	}
	if raw := platformFromUserAgent(header); raw != "" {
		return proxy.CanonicalPlatform(raw)
	}
	return proxy.CanonicalPlatform("JSON")
}

// platformFromUserAgent maps a client User-Agent (and Accept header) onto a
// Sub-Store platform name. The order of the checks matches upstream
// getPlatformFromUserAgent so detection is consistent.
func platformFromUserAgent(header http.Header) string {
	ua := header.Get("User-Agent")
	lower := strings.ToLower(ua)
	accept := strings.ToLower(header.Get("Accept"))

	switch {
	case strings.Contains(ua, "Quantumult%20X"):
		return "QX"
	case strings.Contains(lower, "egern"):
		return "Egern"
	case strings.Contains(ua, "Surfboard"):
		return "Surfboard"
	case strings.Contains(ua, "Surge Mac"):
		return "SurgeMac"
	case strings.Contains(ua, "Surge"):
		return "Surge"
	case strings.Contains(ua, "Decar"), strings.Contains(ua, "Loon"):
		return "Loon"
	case strings.Contains(ua, "Shadowrocket"):
		return "Shadowrocket"
	case strings.Contains(ua, "Stash"):
		return "Stash"
	case lower == "meta",
		strings.Contains(lower, "clash") && strings.Contains(lower, "meta"),
		strings.Contains(lower, "clash-verge"),
		strings.Contains(lower, "flclash"):
		return "ClashMeta"
	case strings.Contains(lower, "clash"):
		return "Clash"
	case strings.Contains(lower, "v2ray"):
		return "V2Ray"
	case strings.Contains(lower, "sing-box"), strings.Contains(lower, "singbox"):
		return "sing-box"
	case strings.HasPrefix(accept, "application/json"):
		return "JSON"
	default:
		return "V2Ray"
	}
}

// downloadFailure is the message Sub-Store returns when producing a download
// fails.
func downloadFailure(kind, name string) string {
	switch kind {
	case storage.KindCollection:
		return "Failed to download collection: " + name
	case storage.KindFile:
		return "Failed to download file: " + name
	default:
		return "Failed to download subscription: " + name
	}
}

// handleFileDownload serves GET /api/file/{name}, returning a file's content so
// it can be referenced from mihomo configs and other files.
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request, name string) {
	doc, err := s.deps.Resources.Get(r.Context(), storage.KindFile, name)
	if errors.Is(err, storage.ErrResourceNotFound) {
		restNotFound(w, "RESOURCE_NOT_FOUND", fmt.Sprintf("File %s does not exist!", name))
		return
	}
	if err != nil {
		s.logError("load file", err, "name", name)
		restInternalError(w, "INTERNAL_SERVER_ERROR", downloadFailure(storage.KindFile, name), err.Error())
		return
	}

	var file fileDocument
	if err := json.Unmarshal([]byte(doc.Data), &file); err != nil {
		restInternalError(w, "INTERNAL_SERVER_ERROR", downloadFailure(storage.KindFile, name), err.Error())
		return
	}

	content, err := s.resolveFileContent(r.Context(), file, r.URL.Query().Get("noCache") != "")
	if err != nil {
		restInternalError(w, "INTERNAL_SERVER_ERROR", downloadFailure(storage.KindFile, name), err.Error())
		return
	}
	if len(file.Process) > 0 {
		s.logSkippedProcessors(storage.KindFile, []string{"processors on file downloads"})
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(content))
}

// resolveFileContent returns a file's body, fetching it when the file points at
// a remote URL.
func (s *Server) resolveFileContent(ctx context.Context, file fileDocument, noCache bool) (string, error) {
	if file.Source == "local" || file.URL == "" {
		return file.Content, nil
	}
	if s.deps.Fetcher == nil {
		return "", errors.New("no fetcher configured; cannot fetch remote file")
	}
	result, err := s.deps.Fetcher.GetCached(ctx, file.URL, "", noCache)
	if err != nil {
		return "", fmt.Errorf("failed to fetch file: %w", err)
	}
	return result.Content, nil
}

// resolveDocument turns a stored document into parsed proxy nodes plus the
// processor chain declared on it.
func (s *Server) resolveDocument(
	ctx context.Context,
	kind string,
	raw json.RawMessage,
	seen map[string]bool,
	noCache bool,
) ([]proxy.Node, []processStep, error) {
	switch kind {
	case "sub", "subs":
		var doc subDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, nil, errors.New("invalid subscription data")
		}
		content, err := s.subscriptionContent(ctx, doc, noCache)
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
			return nil, nil, errors.New("invalid collection data")
		}
		var nodes []proxy.Node
		for _, member := range doc.Subscriptions {
			if seen[member] {
				return nil, nil, fmt.Errorf("collection has a circular reference: %s", member)
			}
			seen[member] = true
			memberDoc, err := s.deps.Resources.Get(ctx, storage.KindSub, member)
			if err != nil {
				return nil, nil, fmt.Errorf("collection references missing subscription %s", member)
			}
			memberNodes, _, err := s.resolveDocument(ctx, "sub", json.RawMessage(memberDoc.Data), seen, noCache)
			if err != nil {
				return nil, nil, err
			}
			nodes = append(nodes, memberNodes...)
		}
		return nodes, doc.Process, nil

	case "file", "files":
		var doc fileDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, nil, errors.New("invalid file data")
		}
		nodes, err := proxy.ParseSubscription(doc.Content)
		if err != nil {
			return nil, nil, err
		}
		return nodes, doc.Process, nil

	default:
		return nil, nil, fmt.Errorf("unsupported resource type: %s", kind)
	}
}

// subscriptionContent returns the subscription body, fetching it when the
// subscription is remote.
func (s *Server) subscriptionContent(ctx context.Context, doc subDocument, noCache bool) (string, error) {
	if doc.Source == "local" || doc.URL == "" {
		return doc.Content, nil
	}
	if s.deps.Fetcher == nil {
		return "", errors.New("no fetcher configured; cannot fetch remote subscription")
	}
	result, err := s.deps.Fetcher.GetCached(ctx, doc.URL, doc.UA, noCache)
	if err != nil {
		return "", fmt.Errorf("failed to fetch subscription: %w", err)
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

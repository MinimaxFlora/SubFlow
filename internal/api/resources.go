package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/subflow-org/subflow/internal/storage"
)

// handleListResources returns every document of a kind.
//
//	GET /api/subs, /api/collections, /api/files, /api/artifacts, /api/tokens
func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request, kind string) {
	docs, err := s.deps.Resources.List(r.Context(), kind)
	if err != nil {
		s.logError("list resources", err, "kind", kind)
		restInternal(w, "读取数据失败")
		return
	}

	items := make([]json.RawMessage, 0, len(docs))
	for _, doc := range docs {
		items = append(items, json.RawMessage(doc.Data))
	}
	restOK(w, items)
}

// handleCreateResource stores a new document. The name must be unique within
// the kind, matching the upstream behaviour.
func (s *Server) handleCreateResource(w http.ResponseWriter, r *http.Request, kind string) {
	var body map[string]json.RawMessage
	if !restDecode(w, r, &body) {
		return
	}

	name, err := documentName(body)
	if err != nil {
		restBadRequest(w, "INVALID_NAME", err.Error())
		return
	}

	exists, err := s.deps.Resources.Exists(r.Context(), kind, name)
	if err != nil {
		s.logError("check resource", err, "kind", kind, "name", name)
		restInternal(w, "读取数据失败")
		return
	}
	if exists {
		restConflict(w, "名称已存在")
		return
	}

	if err := s.deps.Resources.Put(r.Context(), kind, name, encodeDocument(body)); err != nil {
		s.logError("create resource", err, "kind", kind, "name", name)
		restInternal(w, "保存数据失败")
		return
	}
	restOK(w, json.RawMessage(encodeDocument(body)))
}

// handleGetResource returns one document.
func (s *Server) handleGetResource(w http.ResponseWriter, r *http.Request, kind, name string) {
	doc, err := s.deps.Resources.Get(r.Context(), kind, name)
	if errors.Is(err, storage.ErrResourceNotFound) {
		restNotFound(w, "资源不存在")
		return
	}
	if err != nil {
		s.logError("get resource", err, "kind", kind, "name", name)
		restInternal(w, "读取数据失败")
		return
	}

	// ?raw=1 returns the stored document untouched for export.
	if r.URL.Query().Get("raw") != "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(doc.Data))
		return
	}
	restOK(w, json.RawMessage(doc.Data))
}

// handleUpdateResource merges the supplied fields into an existing document.
func (s *Server) handleUpdateResource(w http.ResponseWriter, r *http.Request, kind, name string) {
	current, err := s.deps.Resources.Get(r.Context(), kind, name)
	if errors.Is(err, storage.ErrResourceNotFound) {
		restNotFound(w, "资源不存在")
		return
	}
	if err != nil {
		s.logError("get resource for update", err, "kind", kind, "name", name)
		restInternal(w, "读取数据失败")
		return
	}

	var patch map[string]json.RawMessage
	if !restDecode(w, r, &patch) {
		return
	}

	merged := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(current.Data), &merged); err != nil {
		s.logError("decode stored resource", err, "kind", kind, "name", name)
		restInternal(w, "数据损坏")
		return
	}
	for key, value := range patch {
		merged[key] = value
	}

	newName := name
	if raw, ok := merged["name"]; ok {
		if parsed, err := rawString(raw); err == nil && parsed != "" && parsed != name {
			newName = parsed
		}
	}

	encoded := encodeDocument(merged)
	if newName != name {
		exists, err := s.deps.Resources.Exists(r.Context(), kind, newName)
		if err != nil {
			s.logError("check rename target", err, "kind", kind, "name", newName)
			restInternal(w, "读取数据失败")
			return
		}
		if exists {
			restConflict(w, "名称已存在")
			return
		}
		if err := s.deps.Resources.Put(r.Context(), kind, newName, encoded); err != nil {
			s.logError("create renamed resource", err, "kind", kind, "name", newName)
			restInternal(w, "保存数据失败")
			return
		}
		if _, err := s.deps.Resources.Delete(r.Context(), kind, name); err != nil {
			s.logError("remove old resource", err, "kind", kind, "name", name)
			restInternal(w, "保存数据失败")
			return
		}
		restOK(w, json.RawMessage(encoded))
		return
	}

	if err := s.deps.Resources.Put(r.Context(), kind, name, encoded); err != nil {
		s.logError("update resource", err, "kind", kind, "name", name)
		restInternal(w, "保存数据失败")
		return
	}
	restOK(w, json.RawMessage(encoded))
}

// handleDeleteResource removes a document.
func (s *Server) handleDeleteResource(w http.ResponseWriter, r *http.Request, kind, name string) {
	ok, err := s.deps.Resources.Delete(r.Context(), kind, name)
	if err != nil {
		s.logError("delete resource", err, "kind", kind, "name", name)
		restInternal(w, "删除数据失败")
		return
	}
	if !ok {
		restNotFound(w, "资源不存在")
		return
	}
	restOK(w, map[string]any{})
}

// handleReorderResources rewrites the stored order from a list of documents or
// bare names, matching the upstream sort endpoints.
func (s *Server) handleReorderResources(w http.ResponseWriter, r *http.Request, kind string) {
	var raw json.RawMessage
	if !restDecode(w, r, &raw) {
		return
	}

	names, err := extractNames(raw)
	if err != nil {
		restBadRequest(w, "INVALID_BODY", err.Error())
		return
	}
	if err := s.deps.Resources.Reorder(r.Context(), kind, names); err != nil {
		s.logError("reorder resources", err, "kind", kind)
		restInternal(w, "保存排序失败")
		return
	}
	restOK(w, map[string]any{})
}

// handleWholeFiles returns files together with their content, used by the file
// list so it can open editors without an extra round trip.
func (s *Server) handleWholeFiles(w http.ResponseWriter, r *http.Request) {
	docs, err := s.deps.Resources.List(r.Context(), storage.KindFile)
	if err != nil {
		s.logError("list whole files", err)
		restInternal(w, "读取文件失败")
		return
	}

	items := make([]json.RawMessage, 0, len(docs))
	for _, doc := range docs {
		items = append(items, json.RawMessage(doc.Data))
	}
	restOK(w, items)
}

// documentName extracts the required "name" field from a document body.
func documentName(body map[string]json.RawMessage) (string, error) {
	raw, ok := body["name"]
	if !ok {
		return "", errors.New("缺少 name 字段")
	}
	name, err := rawString(raw)
	if err != nil {
		return "", errors.New("name 字段必须是字符串")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("name 不能为空")
	}
	return name, nil
}

func rawString(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

// encodeDocument serialises a document without HTML escaping so stored names
// and regexes round-trip byte for byte.
func encodeDocument(body map[string]json.RawMessage) string {
	encoded, err := marshalNoEscape(body)
	if err != nil {
		// Bodies originate from valid JSON, so encoding cannot fail here.
		return "{}"
	}
	return string(encoded)
}

// extractNames accepts either [{"name":...}, ...] or ["name", ...].
func extractNames(raw json.RawMessage) ([]string, error) {
	var objects []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &objects); err == nil {
		names := make([]string, 0, len(objects))
		for _, obj := range objects {
			name, err := documentName(obj)
			if err != nil {
				return nil, errors.New("排序列表中的每项都必须包含 name")
			}
			names = append(names, name)
		}
		return names, nil
	}

	var names []string
	if err := json.Unmarshal(raw, &names); err == nil {
		return names, nil
	}
	return nil, errors.New("请求体必须是对象数组或名称数组")
}

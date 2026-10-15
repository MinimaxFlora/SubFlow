package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/subflow-org/subflow/internal/storage"
)

const settingsDocument = "global"

// handleSettingsGet returns the stored application settings. An empty object is
// returned on first run so the frontend applies its own defaults.
func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	doc, err := s.deps.Resources.Get(r.Context(), storage.KindSettings, settingsDocument)
	if errors.Is(err, storage.ErrResourceNotFound) {
		restOK(w, map[string]any{})
		return
	}
	if err != nil {
		s.logError("get settings", err)
		restInternal(w, "读取配置失败")
		return
	}
	restOK(w, json.RawMessage(doc.Data))
}

// handleSettingsPatch merges the supplied keys into the stored settings.
func (s *Server) handleSettingsPatch(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if !restDecode(w, r, &patch) {
		return
	}

	merged := map[string]json.RawMessage{}
	if doc, err := s.deps.Resources.Get(r.Context(), storage.KindSettings, settingsDocument); err == nil {
		if err := json.Unmarshal([]byte(doc.Data), &merged); err != nil {
			s.logError("decode stored settings", err)
			restInternal(w, "配置数据损坏")
			return
		}
	} else if !errors.Is(err, storage.ErrResourceNotFound) {
		s.logError("get settings for update", err)
		restInternal(w, "读取配置失败")
		return
	}

	for key, value := range patch {
		merged[key] = value
	}

	if err := s.deps.Resources.Put(r.Context(), storage.KindSettings, settingsDocument, encodeDocument(merged)); err != nil {
		s.logError("update settings", err)
		restInternal(w, "保存配置失败")
		return
	}
	restOK(w, map[string]any{})
}

// handleStorageExport returns a portable snapshot of every stored document. The
// body is the raw snapshot (not wrapped in the response envelope) so it can be
// saved to a file and posted back to handleStorageImport unchanged, matching the
// Sub-Store backup format.
func (s *Server) handleStorageExport(w http.ResponseWriter, r *http.Request) {
	snapshot := map[string]json.RawMessage{}

	for _, collection := range resourceCollections {
		docs, err := s.deps.Resources.List(r.Context(), collection.Kind)
		if err != nil {
			s.logError("export resources", err, "kind", collection.Kind)
			restInternal(w, "导出数据失败")
			return
		}
		items := map[string]json.RawMessage{}
		for _, doc := range docs {
			items[doc.Name] = json.RawMessage(doc.Data)
		}
		encoded, err := marshalNoEscape(items)
		if err != nil {
			s.logError("encode export section", err, "kind", collection.Kind)
			restInternal(w, "导出数据失败")
			return
		}
		snapshot[collection.Segment] = encoded
	}

	// Settings are always present so a restored backup passes validation even on
	// a fresh install.
	settings := json.RawMessage("{}")
	if doc, err := s.deps.Resources.Get(r.Context(), storage.KindSettings, settingsDocument); err == nil {
		settings = json.RawMessage(doc.Data)
	}
	snapshot["settings"] = settings

	body, err := marshalNoEscape(snapshot)
	if err != nil {
		s.logError("encode storage export", err)
		restInternal(w, "导出数据失败")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="subflow-storage.json"`)
	_, _ = w.Write(body)
}

// handleStorageImport replaces the entire store with a previously exported
// snapshot. The frontend posts {content: <backup>}, where <backup> is either a
// Base64-encoded or plain JSON snapshot; a bare snapshot body is also accepted.
// Unknown sections are ignored.
func (s *Server) handleStorageImport(w http.ResponseWriter, r *http.Request) {
	var body map[string]json.RawMessage
	if !restDecode(w, r, &body) {
		return
	}

	snapshot, err := storageSnapshot(body)
	if err != nil {
		restBadRequest(w, "INVALID_BACKUP_DATA", "备份文件校验失败, 无法还原")
		return
	}

	settingsRaw, ok := snapshot["settings"]
	if !ok {
		restBadRequest(w, "INVALID_BACKUP_DATA", "备份文件应该至少包含 settings 字段")
		return
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(settingsRaw, &settings); err != nil {
		restBadRequest(w, "INVALID_BACKUP_DATA", "settings 字段格式不正确")
		return
	}

	// Restoring replaces the store, so existing documents not present in the
	// snapshot are removed first.
	for _, collection := range resourceCollections {
		existing, err := s.deps.Resources.List(r.Context(), collection.Kind)
		if err != nil {
			s.logError("list before restore", err, "kind", collection.Kind)
			restInternal(w, "导入数据失败")
			return
		}
		for _, doc := range existing {
			if _, err := s.deps.Resources.Delete(r.Context(), collection.Kind, doc.Name); err != nil {
				s.logError("clear before restore", err, "kind", collection.Kind, "name", doc.Name)
				restInternal(w, "导入数据失败")
				return
			}
		}

		raw, ok := snapshot[collection.Segment]
		if !ok {
			continue
		}
		var items map[string]json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			restBadRequest(w, "INVALID_BACKUP_DATA", "备份文件校验失败, 无法还原")
			return
		}
		for name, value := range items {
			doc := map[string]json.RawMessage{}
			if err := json.Unmarshal(value, &doc); err != nil {
				continue
			}
			doc["name"] = mustJSON(name)
			if err := s.deps.Resources.Put(r.Context(), collection.Kind, name, encodeDocument(doc)); err != nil {
				s.logError("import resource", err, "kind", collection.Kind, "name", name)
				restInternal(w, "导入数据失败")
				return
			}
		}
	}

	if err := s.deps.Resources.Put(r.Context(), storage.KindSettings, settingsDocument, encodeDocument(settings)); err != nil {
		s.logError("import settings", err)
		restInternal(w, "导入数据失败")
		return
	}
	restOK(w, map[string]any{})
}

// storageSnapshot resolves the snapshot from either a {content: ...} wrapper or
// a bare snapshot body. The content string is Base64-decoded when possible and
// otherwise parsed as plain JSON.
func storageSnapshot(body map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	raw, ok := body["content"]
	if !ok {
		if len(body) == 0 {
			return nil, errors.New("empty backup")
		}
		return body, nil
	}

	var content string
	if err := json.Unmarshal(raw, &content); err != nil {
		return nil, err
	}

	if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(content)); err == nil {
		var snapshot map[string]json.RawMessage
		if err := json.Unmarshal(decoded, &snapshot); err == nil {
			return snapshot, nil
		}
	}

	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// marshalNoEscape serialises a value without HTML escaping so stored names and
// regexes round-trip byte for byte.
func marshalNoEscape(value any) (json.RawMessage, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return json.RawMessage(strings.TrimRight(buf.String(), "\n")), nil
}

func mustJSON(value string) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return json.RawMessage(encoded)
}

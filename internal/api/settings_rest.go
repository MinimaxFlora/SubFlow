package api

import (
	"encoding/json"
	"errors"
	"net/http"

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

// handleStorageExport returns a portable snapshot of every stored document.
func (s *Server) handleStorageExport(w http.ResponseWriter, r *http.Request) {
	snapshot := map[string]map[string]json.RawMessage{}

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
		snapshot[collection.Segment] = items
	}

	if doc, err := s.deps.Resources.Get(r.Context(), storage.KindSettings, settingsDocument); err == nil {
		settings := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(doc.Data), &settings); err != nil {
			s.logError("decode stored settings", err)
			restInternal(w, "配置数据损坏")
			return
		}
		snapshot["settings"] = settings
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="subflow-storage.json"`)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(map[string]any{"status": "success", "data": snapshot}); err != nil {
		s.logError("write storage export", err)
	}
}

// handleStorageImport replaces stored documents with a previously exported
// snapshot. Unknown sections are ignored.
func (s *Server) handleStorageImport(w http.ResponseWriter, r *http.Request) {
	var body map[string]json.RawMessage
	if !restDecode(w, r, &body) {
		return
	}

	for plural, raw := range body {
		kind, ok := pluralKinds[plural]
		if !ok && plural != "settings" {
			continue
		}
		if plural == "settings" {
			kind = storage.KindSettings
		}

		var items map[string]json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			restBadRequest(w, "INVALID_BODY", "数据格式不正确")
			return
		}
		for name, value := range items {
			doc := map[string]json.RawMessage{}
			if err := json.Unmarshal(value, &doc); err != nil {
				continue
			}
			doc["name"] = mustJSON(name)
			if err := s.deps.Resources.Put(r.Context(), kind, name, encodeDocument(doc)); err != nil {
				s.logError("import resource", err, "kind", kind, "name", name)
				restInternal(w, "导入数据失败")
				return
			}
		}
	}
	restOK(w, map[string]any{})
}

func mustJSON(value string) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return json.RawMessage(encoded)
}

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/subflow-org/subflow/internal/storage"
)

// resourceLabels names each kind the way the Sub-Store API refers to it in
// messages ("Subscription", "Collection", ...).
var resourceLabels = map[string]string{
	storage.KindSub:        "Subscription",
	storage.KindCollection: "Collection",
	storage.KindFile:       "File",
	storage.KindArtifact:   "Artifact",
	storage.KindToken:      "Share",
}

func resourceLabel(kind string) string {
	if label, ok := resourceLabels[kind]; ok {
		return label
	}
	return "Resource"
}

// handleListResources returns every document of a kind.
//
//	GET /api/subs, /api/collections, /api/files, /api/artifacts, /api/tokens
//
// Files are listed without their content, matching Sub-Store's /api/files; the
// /api/wholeFiles route keeps the content.
func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request, kind string, stripContent bool) {
	docs, err := s.deps.Resources.List(r.Context(), kind)
	if err != nil {
		s.logError("list resources", err, "kind", kind)
		restInternalError(w, "FAILED_TO_GET_RESOURCES", "Failed to get resources", err.Error())
		return
	}

	items := make([]json.RawMessage, 0, len(docs))
	for _, doc := range docs {
		items = append(items, json.RawMessage(stripFileContent(kind, doc.Data, stripContent)))
	}
	restSuccess(w, items)
}

// stripFileContent removes the "content" field from a file document. Only the
// /api/files list uses this; every other kind is returned untouched.
func stripFileContent(kind, data string, strip bool) string {
	if !strip || kind != storage.KindFile {
		return data
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &doc); err != nil {
		return data
	}
	delete(doc, "content")
	encoded, err := marshalNoEscape(doc)
	if err != nil {
		return data
	}
	return string(encoded)
}

// handleCreateResource stores a new document. Validation, duplicate handling and
// the 201 status mirror Sub-Store's per-kind create helpers.
func (s *Server) handleCreateResource(w http.ResponseWriter, r *http.Request, kind string) {
	var body map[string]json.RawMessage
	if !restDecode(w, r, &body) {
		return
	}

	label := resourceLabel(kind)
	name, hasName := documentName(body)

	switch kind {
	case storage.KindSub, storage.KindCollection:
		if strings.Contains(name, "/") {
			restRequestInvalid(w, "INVALID_NAME", fmt.Sprintf("%s %s is invalid", label, name))
			return
		}
	case storage.KindFile:
		// Sub-Store names a file after the current millisecond timestamp when
		// the client did not supply one.
		if !hasName {
			name = strconv.FormatInt(time.Now().UnixMilli(), 10)
			body["name"] = mustJSON(name)
		}
	case storage.KindArtifact:
		if !validArtifactName(name) {
			restRequestInvalid(w, "INVALID_ARTIFACT_NAME", fmt.Sprintf("Artifact name %s is invalid.", name))
			return
		}
	}

	exists, err := s.deps.Resources.Exists(r.Context(), kind, name)
	if err != nil {
		s.logError("check resource", err, "kind", kind, "name", name)
		restInternalError(w, "FAILED_TO_CREATE_RESOURCE", "Failed to create resource", err.Error())
		return
	}
	if exists {
		restRequestInvalid(w, "DUPLICATE_KEY", duplicateMessage(kind, name, hasName))
		return
	}

	if err := s.deps.Resources.Put(r.Context(), kind, name, encodeDocument(body)); err != nil {
		s.logError("create resource", err, "kind", kind, "name", name)
		restInternalError(w, "FAILED_TO_CREATE_RESOURCE", "Failed to create resource", err.Error())
		return
	}
	restCreated(w, json.RawMessage(encodeDocument(body)))
}

// duplicateMessage reproduces the short duplicate errors from the upstream
// create helpers. Files are the one place Sub-Store answers in Chinese.
func duplicateMessage(kind, name string, hasName bool) string {
	label := resourceLabel(kind)
	if kind == storage.KindFile {
		if hasName {
			return fmt.Sprintf("已存在 name 为 %s 的文件", name)
		}
		return "无法同时创建相同的文件 可稍后重试"
	}
	return fmt.Sprintf("%s %s already exists.", label, name)
}

// handleGetResource returns one document as JSON.
//
//	GET /api/sub/{name}, /api/collection/{name}, /api/wholeFile/{name},
//	    /api/artifact/{name}
func (s *Server) handleGetResource(w http.ResponseWriter, r *http.Request, segment, kind, name string) {
	doc, err := s.deps.Resources.Get(r.Context(), kind, name)
	if errors.Is(err, storage.ErrResourceNotFound) {
		writeGetNotFound(w, segment, kind, name)
		return
	}
	if err != nil {
		s.logError("get resource", err, "kind", kind, "name", name)
		restInternalError(w, "FAILED_TO_GET_RESOURCE", "Failed to get resource", err.Error())
		return
	}

	// ?raw=1 returns the stored document untouched for export, with the same
	// attachment filename Sub-Store uses.
	if r.URL.Query().Get("raw") != "" {
		writeRawDocument(w, segment, kind, name, doc.Data)
		return
	}
	restSuccess(w, json.RawMessage(doc.Data))
}

// writeGetNotFound reproduces each kind's not-found envelope. Sub-Store passes
// 404 as the error details and relies on failed()'s default status for
// subscriptions, collections and whole files, so those answer with HTTP 500.
func writeGetNotFound(w http.ResponseWriter, segment, kind, name string) {
	switch {
	case segment == "wholeFile":
		restFailed(w, errStatusDefault, "FILE_NOT_FOUND", errTypeNotFound,
			fmt.Sprintf("File %s does not exist", name), 404)
	case kind == storage.KindSub:
		restFailed(w, errStatusDefault, "SUBSCRIPTION_NOT_FOUND", errTypeNotFound,
			fmt.Sprintf("Subscription %s does not exist", name), 404)
	case kind == storage.KindCollection:
		restFailed(w, errStatusDefault, "SUBSCRIPTION_NOT_FOUND", errTypeNotFound,
			fmt.Sprintf("Collection %s does not exist", name), 404)
	case kind == storage.KindArtifact:
		restNotFound(w, "RESOURCE_NOT_FOUND", fmt.Sprintf("Artifact %s does not exist!", name))
	default:
		restNotFound(w, "RESOURCE_NOT_FOUND", fmt.Sprintf("%s %s does not exist!", resourceLabel(kind), name))
	}
}

// writeRawDocument streams a stored document with the upstream attachment
// filename so it can be saved and re-imported.
func writeRawDocument(w http.ResponseWriter, segment, kind, name, data string) {
	filename := fmt.Sprintf("sub-store_%s_%s_%s.json", rawFilenameKind(segment, kind), name, formatDateTime(time.Now()))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Expose-Headers", "content-disposition")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	_, _ = w.Write([]byte(data))
}

func rawFilenameKind(segment, kind string) string {
	if segment == "wholeFile" {
		return "file"
	}
	if label, ok := resourceLabels[kind]; ok {
		return strings.ToLower(label)
	}
	return kind
}

// handleUpdateResource merges the supplied fields into an existing document and
// cascades renames to the documents that reference it by name.
func (s *Server) handleUpdateResource(w http.ResponseWriter, r *http.Request, kind, name string) {
	current, err := s.deps.Resources.Get(r.Context(), kind, name)
	if errors.Is(err, storage.ErrResourceNotFound) {
		if kind == storage.KindArtifact {
			// Upstream reports a duplicate here, not a missing resource.
			restRequestInvalid(w, "DUPLICATE_KEY", fmt.Sprintf("Artifact %s already exists.", name))
			return
		}
		restNotFound(w, "RESOURCE_NOT_FOUND", fmt.Sprintf("%s %s does not exist!", resourceLabel(kind), name))
		return
	}
	if err != nil {
		s.logError("get resource for update", err, "kind", kind, "name", name)
		restInternalError(w, "FAILED_TO_UPDATE_RESOURCE", "Failed to update resource", err.Error())
		return
	}

	var patch map[string]json.RawMessage
	if !restDecode(w, r, &patch) {
		return
	}

	merged := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(current.Data), &merged); err != nil {
		s.logError("decode stored resource", err, "kind", kind, "name", name)
		restInternalError(w, "FAILED_TO_UPDATE_RESOURCE", "Failed to update resource", err.Error())
		return
	}
	for key, value := range patch {
		merged[key] = value
	}

	newName := name
	if raw, ok := merged["name"]; ok {
		if parsed, err := rawString(raw); err == nil && parsed != name {
			newName = parsed
		}
	}

	if kind == storage.KindArtifact && !validArtifactName(newName) {
		restRequestInvalid(w, "INVALID_ARTIFACT_NAME", fmt.Sprintf("Artifact name %s is invalid.", newName))
		return
	}

	encoded := encodeDocument(merged)
	if newName != name {
		exists, err := s.deps.Resources.Exists(r.Context(), kind, newName)
		if err != nil {
			s.logError("check rename target", err, "kind", kind, "name", newName)
			restInternalError(w, "FAILED_TO_UPDATE_RESOURCE", "Failed to update resource", err.Error())
			return
		}
		if exists {
			restRequestInvalid(w, "DUPLICATE_KEY", duplicateMessage(kind, newName, true))
			return
		}
		if err := s.deps.Resources.Put(r.Context(), kind, newName, encoded); err != nil {
			s.logError("create renamed resource", err, "kind", kind, "name", newName)
			restInternalError(w, "FAILED_TO_UPDATE_RESOURCE", "Failed to update resource", err.Error())
			return
		}
		if _, err := s.deps.Resources.Delete(r.Context(), kind, name); err != nil {
			s.logError("remove old resource", err, "kind", kind, "name", name)
			restInternalError(w, "FAILED_TO_UPDATE_RESOURCE", "Failed to update resource", err.Error())
			return
		}
		s.cascadeRename(r, kind, name, newName)
	} else if err := s.deps.Resources.Put(r.Context(), kind, name, encoded); err != nil {
		s.logError("update resource", err, "kind", kind, "name", name)
		restInternalError(w, "FAILED_TO_UPDATE_RESOURCE", "Failed to update resource", err.Error())
		return
	}
	restSuccess(w, json.RawMessage(encoded))
}

// handleDeleteResource removes a document.
func (s *Server) handleDeleteResource(w http.ResponseWriter, r *http.Request, kind, name string) {
	ok, err := s.deps.Resources.Delete(r.Context(), kind, name)
	if err != nil {
		s.logError("delete resource", err, "kind", kind, "name", name)
		restInternalError(w, "FAILED_TO_DELETE_RESOURCE", "Failed to delete resource", err.Error())
		return
	}
	if !ok {
		// Sub-Store throws ResourceNotFoundError from its delete helpers without
		// an explicit status, so the response is the default 500.
		restNotFoundStatusMessage(w, "RESOURCE_NOT_FOUND",
			fmt.Sprintf("%s %s does not exist!", resourceLabel(kind), name))
		return
	}
	if kind == storage.KindSub {
		s.removeSubFromCollections(r, name)
	}
	restSuccess(w, nil)
}

// restNotFoundStatusMessage writes a ResourceNotFoundError that relies on the
// default status, matching the upstream delete paths.
func restNotFoundStatusMessage(w http.ResponseWriter, code, message string) {
	restFailed(w, errStatusDefault, code, errTypeNotFound, message, nil)
}

// handleReplaceResources replaces the whole collection with the request body,
// matching the PUT routes (PUT /api/subs, /api/collections, /api/files,
// /api/artifacts).
func (s *Server) handleReplaceResources(w http.ResponseWriter, r *http.Request, kind string) {
	var items []map[string]json.RawMessage
	if !restDecode(w, r, &items) {
		return
	}

	existing, err := s.deps.Resources.List(r.Context(), kind)
	if err != nil {
		s.logError("list before replace", err, "kind", kind)
		restInternalError(w, "FAILED_TO_REPLACE_RESOURCES", "Failed to replace resources", err.Error())
		return
	}
	for _, doc := range existing {
		if _, err := s.deps.Resources.Delete(r.Context(), kind, doc.Name); err != nil {
			s.logError("clear before replace", err, "kind", kind, "name", doc.Name)
			restInternalError(w, "FAILED_TO_REPLACE_RESOURCES", "Failed to replace resources", err.Error())
			return
		}
	}
	for _, item := range items {
		name, _ := documentName(item)
		if err := s.deps.Resources.Put(r.Context(), kind, name, encodeDocument(item)); err != nil {
			s.logError("replace resource", err, "kind", kind, "name", name)
			restInternalError(w, "FAILED_TO_REPLACE_RESOURCES", "Failed to replace resources", err.Error())
			return
		}
	}
	restSuccess(w, nil)
}

// handleSortResources implements POST /api/sort/{type}. The body is an ordered
// list of keys (a name, or "type-name-token" for shares). The reordered list is
// returned, matching Sub-Store's sort routes.
func (s *Server) handleSortResources(w http.ResponseWriter, r *http.Request, kind string) {
	var order []string
	if !restDecode(w, r, &order) {
		return
	}

	docs, err := s.deps.Resources.List(r.Context(), kind)
	if err != nil {
		s.logError("list for sort", err, "kind", kind)
		restInternalError(w, "FAILED_TO_SORT_RESOURCES", "Failed to sort resources", err.Error())
		return
	}

	ordered, names := sortResources(kind, docs, order)
	if err := s.deps.Resources.Reorder(r.Context(), kind, names); err != nil {
		s.logError("reorder resources", err, "kind", kind)
		restInternalError(w, "FAILED_TO_SORT_RESOURCES", "Failed to sort resources", err.Error())
		return
	}

	items := make([]json.RawMessage, 0, len(ordered))
	for _, doc := range ordered {
		items = append(items, json.RawMessage(doc.Data))
	}
	restSuccess(w, items)
}

// sortResources orders documents by their key's position in order, appending any
// documents the client did not mention. It returns the ordered documents and the
// matching name list for persistence.
func sortResources(kind string, docs []storage.Resource, order []string) ([]storage.Resource, []string) {
	position := make(map[string]int, len(order))
	for i, key := range order {
		if _, seen := position[key]; !seen {
			position[key] = i
		}
	}
	rank := func(doc storage.Resource) int {
		if i, ok := position[sortKey(kind, doc)]; ok {
			return i
		}
		return len(order)
	}

	ordered := make([]storage.Resource, len(docs))
	copy(ordered, docs)
	stableSortBy(ordered, rank)

	names := make([]string, 0, len(ordered))
	for _, doc := range ordered {
		names = append(names, doc.Name)
	}
	return ordered, names
}

// sortKey is the string Sub-Store sorts a kind by.
func sortKey(kind string, doc storage.Resource) string {
	if kind != storage.KindToken {
		return doc.Name
	}
	var token struct {
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	_ = json.Unmarshal([]byte(doc.Data), &token)
	return fmt.Sprintf("%s-%s-%s", kind, token.Name, token.Token)
}

// stableSortBy is an insertion sort; resource lists are short and this keeps the
// ordering deterministic without pulling in a generic helper.
func stableSortBy(docs []storage.Resource, rank func(storage.Resource) int) {
	for i := 1; i < len(docs); i++ {
		for j := i; j > 0 && rank(docs[j]) < rank(docs[j-1]); j-- {
			docs[j], docs[j-1] = docs[j-1], docs[j]
		}
	}
}

// handleWholeFiles returns files together with their content.
func (s *Server) handleWholeFiles(w http.ResponseWriter, r *http.Request) {
	s.handleListResources(w, r, storage.KindFile, false)
}

// cascadeRename updates the documents that reference a renamed subscription,
// collection or file, mirroring the upstream update helpers.
func (s *Server) cascadeRename(r *http.Request, kind, oldName, newName string) {
	switch kind {
	case storage.KindSub:
		s.mutateDocuments(r, storage.KindCollection, func(doc map[string]json.RawMessage) bool {
			return replaceInStringArray(doc, "subscriptions", oldName, newName)
		})
		s.mutateDocuments(r, storage.KindArtifact, func(doc map[string]json.RawMessage) bool {
			return replaceSource(doc, "subscription", oldName, newName)
		})
		s.mutateDocuments(r, storage.KindFile, func(doc map[string]json.RawMessage) bool {
			return replaceFileSource(doc, "subscription", oldName, newName)
		})
	case storage.KindCollection:
		s.mutateDocuments(r, storage.KindArtifact, func(doc map[string]json.RawMessage) bool {
			return replaceSource(doc, "collection", oldName, newName)
		})
		s.mutateDocuments(r, storage.KindFile, func(doc map[string]json.RawMessage) bool {
			return replaceFileSource(doc, "collection", oldName, newName)
		})
	case storage.KindFile:
		s.mutateDocuments(r, storage.KindArtifact, func(doc map[string]json.RawMessage) bool {
			return replaceSource(doc, "file", oldName, newName)
		})
	}
}

// removeSubFromCollections drops a deleted subscription from every collection.
func (s *Server) removeSubFromCollections(r *http.Request, name string) {
	s.mutateDocuments(r, storage.KindCollection, func(doc map[string]json.RawMessage) bool {
		raw, ok := doc["subscriptions"]
		if !ok {
			return false
		}
		members, err := stringArray(raw)
		if err != nil {
			return false
		}
		filtered := make([]string, 0, len(members))
		changed := false
		for _, member := range members {
			if member == name {
				changed = true
				continue
			}
			filtered = append(filtered, member)
		}
		if !changed {
			return false
		}
		doc["subscriptions"] = mustJSONArray(filtered)
		return true
	})
}

// mutateDocuments loads every document of kind, lets edit mutate its JSON, and
// persists the ones that changed.
func (s *Server) mutateDocuments(r *http.Request, kind string, edit func(map[string]json.RawMessage) bool) {
	docs, err := s.deps.Resources.List(r.Context(), kind)
	if err != nil {
		s.logError("cascade list", err, "kind", kind)
		return
	}
	for _, doc := range docs {
		var body map[string]json.RawMessage
		if err := json.Unmarshal([]byte(doc.Data), &body); err != nil {
			continue
		}
		if !edit(body) {
			continue
		}
		if err := s.deps.Resources.Put(r.Context(), kind, doc.Name, encodeDocument(body)); err != nil {
			s.logError("cascade update", err, "kind", kind, "name", doc.Name)
		}
	}
}

// replaceInStringArray rewrites every occurrence of oldName in a string array
// field, reporting whether anything changed.
func replaceInStringArray(doc map[string]json.RawMessage, field, oldName, newName string) bool {
	raw, ok := doc[field]
	if !ok {
		return false
	}
	values, err := stringArray(raw)
	if err != nil {
		return false
	}
	changed := false
	for i, value := range values {
		if value == oldName {
			values[i] = newName
			changed = true
		}
	}
	if changed {
		doc[field] = mustJSONArray(values)
	}
	return changed
}

// replaceSource rewrites an artifact's source when its type matches.
func replaceSource(doc map[string]json.RawMessage, artifactType, oldName, newName string) bool {
	var typeValue, source string
	if raw, ok := doc["type"]; ok {
		_ = json.Unmarshal(raw, &typeValue)
	} else {
		return false
	}
	if raw, ok := doc["source"]; ok {
		_ = json.Unmarshal(raw, &source)
	} else {
		return false
	}
	if typeValue != artifactType || source != oldName {
		return false
	}
	doc["source"] = mustJSON(newName)
	return true
}

// replaceFileSource rewrites a file's sourceName when its sourceType matches.
func replaceFileSource(doc map[string]json.RawMessage, sourceType, oldName, newName string) bool {
	var kindValue, sourceName string
	if raw, ok := doc["sourceType"]; ok {
		_ = json.Unmarshal(raw, &kindValue)
	} else {
		return false
	}
	if raw, ok := doc["sourceName"]; ok {
		_ = json.Unmarshal(raw, &sourceName)
	} else {
		return false
	}
	if kindValue != sourceType || sourceName != oldName {
		return false
	}
	doc["sourceName"] = mustJSON(newName)
	return true
}

func stringArray(raw json.RawMessage) ([]string, error) {
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func mustJSONArray(values []string) json.RawMessage {
	encoded, err := marshalNoEscape(values)
	if err != nil {
		return json.RawMessage("[]")
	}
	return encoded
}

// artifactNamePattern is the character set Sub-Store allows for artifact names.
var artifactNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]*$`)

// validArtifactName reports whether an artifact name is usable as a Gist file.
func validArtifactName(name string) bool {
	return artifactNamePattern.MatchString(name) && name != ".sub-store-placeholder"
}

// documentName extracts the "name" field. The second return reports whether the
// field was present, which the file create path uses to decide on a default.
func documentName(body map[string]json.RawMessage) (string, bool) {
	raw, ok := body["name"]
	if !ok {
		return "", false
	}
	name, err := rawString(raw)
	if err != nil {
		return "", true
	}
	return strings.TrimSpace(name), true
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

// formatDateTime renders a timestamp the way Sub-Store names export files.
func formatDateTime(t time.Time) string {
	return t.Format("2006-01-02T15:04:05.000Z")
}

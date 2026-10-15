package api

import (
	"log/slog"
	"net/http"

	"github.com/subflow-org/subflow/internal/storage"
)

// pluralKinds resolves a collection URL segment to a storage kind. "wholeFiles"
// is an alias used when the frontend needs file contents in one call.
var pluralKinds = map[string]string{
	"subs":        storage.KindSub,
	"collections": storage.KindCollection,
	"files":       storage.KindFile,
	"wholeFiles":  storage.KindFile,
	"artifacts":   storage.KindArtifact,
	"tokens":      storage.KindToken,
}

// itemKinds resolves a single-item URL segment to a storage kind.
var itemKinds = map[string]string{
	"sub":        storage.KindSub,
	"collection": storage.KindCollection,
	"file":       storage.KindFile,
	"wholeFile":  storage.KindFile,
	"artifact":   storage.KindArtifact,
	"token":      storage.KindToken,
}

// resourceCollections lists the canonical collection segments, used for export.
var resourceCollections = []struct {
	Segment string
	Kind    string
}{
	{"subs", storage.KindSub},
	{"collections", storage.KindCollection},
	{"files", storage.KindFile},
	{"artifacts", storage.KindArtifact},
	{"tokens", storage.KindToken},
}

// handleResourceCollection dispatches the collection-level resource routes:
//
//	GET    /api/subs            list subscriptions
//	POST   /api/subs            create a subscription
//	PUT    /api/subs            reorder subscriptions
func (s *Server) handleResourceCollection(w http.ResponseWriter, r *http.Request) {
	segment := r.PathValue("type")
	kind, ok := pluralKinds[segment]
	if !ok {
		if segment == "archives" {
			// Archive management is not implemented; the collection is empty.
			restOK(w, []any{})
			return
		}
		restNotFound(w, "未知的资源类型: "+segment)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if segment == "wholeFiles" {
			s.handleWholeFiles(w, r)
			return
		}
		s.handleListResources(w, r, kind)
	case http.MethodPost:
		s.handleCreateResource(w, r, kind)
	case http.MethodPut:
		s.handleReorderResources(w, r, kind)
	default:
		methodNotAllowed(w, "GET, POST, PUT")
	}
}

// handleResourceItem dispatches the single-resource routes:
//
//	GET    /api/sub/{name}
//	PATCH  /api/sub/{name}
//	DELETE /api/sub/{name}
func (s *Server) handleResourceItem(w http.ResponseWriter, r *http.Request) {
	segment := r.PathValue("type")
	name := r.PathValue("name")
	kind, ok := itemKinds[segment]
	if !ok {
		restNotFound(w, "未知的资源类型: "+segment)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetResource(w, r, kind, name)
	case http.MethodPatch, http.MethodPut:
		s.handleUpdateResource(w, r, kind, name)
	case http.MethodDelete:
		s.handleDeleteResource(w, r, kind, name)
	default:
		methodNotAllowed(w, "GET, PATCH, DELETE")
	}
}

// handleSortResource implements POST /api/sort/{type}.
func (s *Server) handleSortResource(w http.ResponseWriter, r *http.Request) {
	segment := r.PathValue("type")
	if segment == "archives" {
		restOK(w, map[string]any{})
		return
	}
	kind, ok := pluralKinds[segment]
	if !ok {
		if kind, ok = itemKinds[segment]; !ok {
			restNotFound(w, "未知的资源类型: "+segment)
			return
		}
	}
	s.handleReorderResources(w, r, kind)
}

// handlePreviewRoute implements POST /api/preview/{type}.
func (s *Server) handlePreviewRoute(w http.ResponseWriter, r *http.Request) {
	s.handlePreview(w, r, r.PathValue("type"))
}

// handleSubFlowRoute implements GET /api/sub/flow/{name}. Traffic statistics
// are parsed from subscription headers, which SubFlow does not do yet, so every
// subscription reports "noFlow" instead of inventing numbers.
func (s *Server) handleSubFlowRoute(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(`{"status":"noFlow"}`))
}

// handleNotImplemented reports a feature that exists in the upstream API but is
// not available in SubFlow yet, so the frontend shows an honest error.
func (s *Server) handleNotImplemented(feature string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		restFail(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "NotImplemented",
			feature+" 尚未实现")
	}
}

// handleLogsList serves GET /api/logs. SubFlow does not keep an in-memory debug
// log buffer yet, so it reports an empty list rather than fabricating entries.
func (s *Server) handleLogsList(w http.ResponseWriter, _ *http.Request) {
	restOK(w, map[string]any{
		"logs":     []any{},
		"total":    0,
		"maxCount": 0,
	})
}

// handleLogsClear serves DELETE /api/logs. With no buffer there is nothing to
// clear, so it succeeds with the same empty payload.
func (s *Server) handleLogsClear(w http.ResponseWriter, r *http.Request) {
	s.handleLogsList(w, r)
}

// handleDownloadSub serves GET /download/{name}.
func (s *Server) handleDownloadSub(w http.ResponseWriter, r *http.Request) {
	s.handleDownload(w, r, storage.KindSub, r.PathValue("name"))
}

// handleDownloadCollection serves GET /download/collection/{name}.
func (s *Server) handleDownloadCollection(w http.ResponseWriter, r *http.Request) {
	s.handleDownload(w, r, storage.KindCollection, r.PathValue("name"))
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	restFail(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Validation", "请求方法不被支持")
}

// logError records an internal error with context but never exposes it to the
// client.
func (s *Server) logError(message string, err error, args ...any) {
	fields := append([]any{"error", err}, args...)
	slog.Error(message, fields...)
}

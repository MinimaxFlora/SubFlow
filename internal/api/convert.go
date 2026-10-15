package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/subflow-org/subflow/internal/fetch"
	"github.com/subflow-org/subflow/internal/proxy"
)

var errFetcherDisabled = errors.New("remote fetching is not enabled")

// handleListPlatforms returns the supported output targets.
func (s *Server) handleListPlatforms(w http.ResponseWriter, _ *http.Request) {
	respondOK(w, http.StatusOK, proxy.Platforms)
}

type convertRequest struct {
	Content            string            `json:"content"`
	URL                string            `json:"url"`
	UserAgent          string            `json:"userAgent"`
	Platform           string            `json:"platform"`
	Operations         []proxy.Operation `json:"operations"`
	IncludeUnsupported bool              `json:"includeUnsupported"`
}

// handleConvert parses subscription content, applies operations and renders
// the requested target platform.
func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
	var req convertRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Platform == "" {
		respondError(w, http.StatusBadRequest, "MISSING_PLATFORM", "platform is required")
		return
	}
	if !proxy.IsPlatform(req.Platform) {
		respondError(w, http.StatusBadRequest, "UNKNOWN_PLATFORM", "unsupported platform: "+req.Platform)
		return
	}

	content := req.Content
	source := ""
	if strings.TrimSpace(content) == "" && strings.TrimSpace(req.URL) != "" {
		result, err := s.retrieve(r, req.URL, req.UserAgent)
		if err != nil {
			respondError(w, http.StatusBadGateway, "FETCH_FAILED", err.Error())
			return
		}
		content = result.Content
		source = result.FinalURL
	}

	result, err := proxy.Convert(proxy.ConvertOptions{
		Content:            proxy.NormalizeContent(content),
		Operations:         req.Operations,
		Platform:           req.Platform,
		IncludeUnsupported: req.IncludeUnsupported,
	})
	if err != nil {
		respondError(w, http.StatusBadRequest, "CONVERT_FAILED", err.Error())
		return
	}
	if source != "" {
		result.Source = source
	}
	respondOK(w, http.StatusOK, result)
}

type fetchRequest struct {
	URL       string `json:"url"`
	UserAgent string `json:"userAgent"`
}

// handleFetch retrieves a remote subscription and reports how many nodes it
// contains without rendering a target format.
func (s *Server) handleFetch(w http.ResponseWriter, r *http.Request) {
	var req fetchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		respondError(w, http.StatusBadRequest, "MISSING_URL", "url is required")
		return
	}

	result, err := s.retrieve(r, req.URL, req.UserAgent)
	if err != nil {
		respondError(w, http.StatusBadGateway, "FETCH_FAILED", err.Error())
		return
	}

	nodes, parseErr := proxy.ParseSubscription(proxy.NormalizeContent(result.Content))
	respondOK(w, http.StatusOK, map[string]any{
		"finalUrl":    result.FinalURL,
		"contentType": result.ContentType,
		"bytes":       result.Bytes,
		"nodes":       len(nodes),
		"preview":     result.Content,
		"parseError":  errorString(parseErr),
	})
}

func (s *Server) retrieve(r *http.Request, rawURL, userAgent string) (fetch.Result, error) {
	if s.deps.Fetcher == nil {
		return fetch.Result{}, errFetcherDisabled
	}
	return s.deps.Fetcher.Get(r.Context(), rawURL, userAgent)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

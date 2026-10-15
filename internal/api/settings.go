package api

import (
	"net/http"
)

// handleGetSettings returns all stored settings as a flat key/value map.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	keys, err := s.deps.Settings.Keys(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "SETTINGS_READ_FAILED", err.Error())
		return
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		v, ok, err := s.deps.Settings.Get(r.Context(), k)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "SETTINGS_READ_FAILED", err.Error())
			return
		}
		if ok {
			out[k] = v
		}
	}
	respondOK(w, http.StatusOK, out)
}

// handleUpdateSettings upserts the provided key/value pairs.
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var payload map[string]string
	if !decodeJSON(w, r, &payload) {
		return
	}
	for k, v := range payload {
		if k == "" {
			respondError(w, http.StatusBadRequest, "INVALID_KEY", "setting key must not be empty")
			return
		}
		if err := s.deps.Settings.Set(r.Context(), k, v); err != nil {
			respondError(w, http.StatusInternalServerError, "SETTINGS_WRITE_FAILED", err.Error())
			return
		}
	}
	s.handleGetSettings(w, r)
}

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/subflow-org/subflow/internal/model"
	"github.com/subflow-org/subflow/internal/proxy"
	"github.com/subflow-org/subflow/internal/storage"
)

type subscriptionDTO struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	DisplayName string          `json:"displayName"`
	Source      string          `json:"source"`
	URL         string          `json:"url"`
	Enabled     bool            `json:"enabled"`
	Sort        int             `json:"sort"`
	Process     json.RawMessage `json:"process"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

func toSubscriptionDTO(sub model.Subscription) subscriptionDTO {
	process := sub.Process
	if len(process) == 0 {
		process = json.RawMessage("[]")
	}
	return subscriptionDTO{
		ID:          sub.ID,
		Name:        sub.Name,
		DisplayName: sub.DisplayName,
		Source:      sub.Source,
		URL:         sub.URL,
		Enabled:     sub.Enabled,
		Sort:        sub.Sort,
		Process:     process,
		CreatedAt:   sub.CreatedAt,
		UpdatedAt:   sub.UpdatedAt,
	}
}

type subscriptionInput struct {
	Name        string          `json:"name"`
	DisplayName string          `json:"displayName"`
	Source      string          `json:"source"`
	URL         string          `json:"url"`
	Content     string          `json:"content"`
	Enabled     *bool           `json:"enabled"`
	Sort        *int            `json:"sort"`
	Process     json.RawMessage `json:"process"`
}

// validate normalises and checks the input, returning a user-facing error.
func (in *subscriptionInput) validate() (string, string) {
	in.Name = strings.TrimSpace(in.Name)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Source = strings.TrimSpace(in.Source)
	in.URL = strings.TrimSpace(in.URL)

	if in.Name == "" {
		return "INVALID_NAME", "name is required"
	}
	if in.Source == "" {
		in.Source = model.SourceRemote
	}
	if !model.ValidateSource(in.Source) {
		return "INVALID_SOURCE", "source must be 'remote' or 'local'"
	}
	if in.Source == model.SourceRemote && in.URL == "" {
		return "INVALID_URL", "url is required for remote subscriptions"
	}
	if len(in.Process) > 0 && !json.Valid(in.Process) {
		return "INVALID_PROCESS", "process must be valid JSON"
	}
	return "", ""
}

func (s *Server) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	subs, err := s.deps.Subscriptions.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
		return
	}
	out := make([]subscriptionDTO, 0, len(subs))
	for _, sub := range subs {
		out = append(out, toSubscriptionDTO(sub))
	}
	respondOK(w, http.StatusOK, out)
}

func (s *Server) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	var in subscriptionInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if code, msg := in.validate(); code != "" {
		respondError(w, http.StatusBadRequest, code, msg)
		return
	}

	sub := model.Subscription{
		Name:        in.Name,
		DisplayName: in.DisplayName,
		Source:      in.Source,
		URL:         in.URL,
		Content:     in.Content,
		Enabled:     true,
		Process:     in.Process,
	}
	if in.Enabled != nil {
		sub.Enabled = *in.Enabled
	}
	if in.Sort != nil {
		sub.Sort = *in.Sort
	}

	created, err := s.deps.Subscriptions.Create(r.Context(), sub)
	if err != nil {
		respondStorageError(w, err)
		return
	}
	respondOK(w, http.StatusCreated, toSubscriptionDTO(created))
}

func (s *Server) handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	sub, err := s.deps.Subscriptions.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		respondStorageError(w, err)
		return
	}
	respondOK(w, http.StatusOK, toSubscriptionDTO(sub))
}

func (s *Server) handleUpdateSubscription(w http.ResponseWriter, r *http.Request) {
	var in subscriptionInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if code, msg := in.validate(); code != "" {
		respondError(w, http.StatusBadRequest, code, msg)
		return
	}

	id := r.PathValue("id")
	sub := model.Subscription{
		ID:          id,
		Name:        in.Name,
		DisplayName: in.DisplayName,
		Source:      in.Source,
		URL:         in.URL,
		Content:     in.Content,
		Enabled:     true,
		Process:     in.Process,
	}
	if in.Enabled != nil {
		sub.Enabled = *in.Enabled
	}
	if in.Sort != nil {
		sub.Sort = *in.Sort
	}

	updated, err := s.deps.Subscriptions.Update(r.Context(), sub)
	if err != nil {
		respondStorageError(w, err)
		return
	}
	respondOK(w, http.StatusOK, toSubscriptionDTO(updated))
}

func (s *Server) handleDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Subscriptions.Delete(r.Context(), r.PathValue("id")); err != nil {
		respondStorageError(w, err)
		return
	}
	respondOK(w, http.StatusOK, map[string]string{"id": r.PathValue("id")})
}

// handleSubscriptionContent returns the resolved node content for a
// subscription: stored content for local sources, a fresh fetch for remote ones.
func (s *Server) handleSubscriptionContent(w http.ResponseWriter, r *http.Request) {
	sub, err := s.deps.Subscriptions.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		respondStorageError(w, err)
		return
	}

	content := sub.Content
	source := ""
	if sub.Source == model.SourceRemote {
		if strings.TrimSpace(sub.URL) == "" {
			respondError(w, http.StatusBadRequest, "MISSING_URL", "remote subscription has no url")
			return
		}
		result, ferr := s.retrieve(r, sub.URL, "")
		if ferr != nil {
			respondError(w, http.StatusBadGateway, "FETCH_FAILED", ferr.Error())
			return
		}
		content = result.Content
		source = result.FinalURL
	}

	nodes, parseErr := proxy.ParseSubscription(proxy.NormalizeContent(content))
	respondOK(w, http.StatusOK, map[string]any{
		"content":    content,
		"nodes":      len(nodes),
		"source":     source,
		"parseError": errorString(parseErr),
	})
}

func respondStorageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		respondError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case errors.Is(err, storage.ErrConflict):
		respondError(w, http.StatusConflict, "CONFLICT", "a resource with the same name already exists")
	default:
		respondError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
	}
}

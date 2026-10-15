package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// The SubFlow HTTP API follows the Sub-Store contract so the Sub-Store
// frontend can talk to it unchanged:
//
//	success: {"status":"success","data":...}
//	failure: {"status":"failed","error":{"code":...,"type":...,"message":...}}
//
// This is intentionally different from the envelope used by the legacy
// /api/subscriptions endpoints, which remain for compatibility.

type restError struct {
	Code    string `json:"code"`
	Type    string `json:"type"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

type restResponse struct {
	Status string     `json:"status"`
	Data   any        `json:"data,omitempty"`
	Error  *restError `json:"error,omitempty"`
}

func writeRest(w http.ResponseWriter, httpStatus int, body restResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(httpStatus)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write rest response", "error", err)
	}
}

// restOK writes a successful response carrying data.
func restOK(w http.ResponseWriter, data any) {
	writeRest(w, http.StatusOK, restResponse{Status: "success", Data: data})
}

// restFail writes a failed response. The error type groups failures the way
// the frontend expects them (Validation, NotFound, Conflict, ...).
func restFail(w http.ResponseWriter, status int, code, errType, message string) {
	writeRest(w, status, restResponse{
		Status: "failed",
		Error:  &restError{Code: code, Type: errType, Message: message},
	})
}

// restBadRequest reports an invalid request body or parameters.
func restBadRequest(w http.ResponseWriter, code, message string) {
	restFail(w, http.StatusBadRequest, code, "Validation", message)
}

// restNotFound reports a missing resource.
func restNotFound(w http.ResponseWriter, message string) {
	restFail(w, http.StatusNotFound, "NOT_FOUND", "NotFound", message)
}

// restConflict reports a duplicate name.
func restConflict(w http.ResponseWriter, message string) {
	restFail(w, http.StatusConflict, "CONFLICT", "Conflict", message)
}

// restInternal reports an unexpected server error without leaking internals.
func restInternal(w http.ResponseWriter, message string) {
	restFail(w, http.StatusInternalServerError, "INTERNAL", "Internal", message)
}

// restDecode reads a JSON body into dst. Unknown fields are accepted because
// the frontend sends richer documents than the backend interprets.
func restDecode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
	if err := dec.Decode(dst); err != nil {
		restBadRequest(w, "INVALID_BODY", "请求体不是合法的 JSON: "+err.Error())
		return false
	}
	return true
}

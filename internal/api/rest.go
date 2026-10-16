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
//	failure: {"status":"failed","error":{"code":...,"type":...,"message":...,"details":...}}
//
// This is intentionally different from the envelope used by the legacy
// /api/subscriptions endpoints, which remain for compatibility.

// Error type names mirror the Sub-Store error classes so the frontend renders
// failures exactly as it does against the upstream backend.
const (
	errTypeRequestInvalid = "RequestInvalidError"
	errTypeNotFound       = "ResourceNotFoundError"
	errTypeInternal       = "InternalServerError"
	errTypeNetwork        = "NetworkError"
)

// errStatusDefault is the status Sub-Store's failed() uses when a route does
// not pass an explicit status code.
const errStatusDefault = http.StatusInternalServerError

type restError struct {
	Code    string `json:"code"`
	Type    string `json:"type"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
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

// restSuccess mirrors Sub-Store's success(res, data): HTTP 200 with the data
// field. A nil data is omitted, matching `success(res)` with no payload.
func restSuccess(w http.ResponseWriter, data any) {
	writeRest(w, http.StatusOK, restResponse{Status: "success", Data: data})
}

// restCreated mirrors success(res, data, 201).
func restCreated(w http.ResponseWriter, data any) {
	writeRest(w, http.StatusCreated, restResponse{Status: "success", Data: data})
}

// restFailed mirrors Sub-Store's failed(res, error, statusCode). Pass
// errStatusDefault to reproduce the routes that rely on the default status.
func restFailed(w http.ResponseWriter, status int, code, errType, message string, details any) {
	writeRest(w, status, restResponse{
		Status: "failed",
		Error: &restError{
			Code:    code,
			Type:    errType,
			Message: message,
			Details: details,
		},
	})
}

// restRequestInvalid mirrors `failed(res, new RequestInvalidError(...))` with
// no explicit status, which Sub-Store answers with HTTP 500.
func restRequestInvalid(w http.ResponseWriter, code, message string) {
	restFailed(w, errStatusDefault, code, errTypeRequestInvalid, message, nil)
}

// restNotFound mirrors the file/artifact not-found paths that pass 404.
func restNotFound(w http.ResponseWriter, code, message string) {
	restFailed(w, http.StatusNotFound, code, errTypeNotFound, message, nil)
}

// restInternalError mirrors an InternalServerError with a human reason, which
// Sub-Store places in the details field.
func restInternalError(w http.ResponseWriter, code, message, reason string) {
	var details any
	if reason != "" {
		details = reason
	}
	restFailed(w, errStatusDefault, code, errTypeInternal, message, details)
}

// restDecode reads a JSON body into dst. Unknown fields are accepted because
// the frontend sends richer documents than the backend interprets.
func restDecode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
	if err := dec.Decode(dst); err != nil {
		restRequestInvalid(w, "INVALID_BODY", "Invalid JSON body: "+err.Error())
		return false
	}
	return true
}

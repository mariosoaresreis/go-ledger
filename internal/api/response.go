package api

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the standard error envelope returned by all endpoints.
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// last-resort: the status code is already sent, nothing we can do
		return
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Code: status, Message: msg})
}

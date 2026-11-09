package api

import (
	"encoding/json"
	"net/http"
)

// ErrorBody is the error envelope from the journey search tech spec §3.
type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// Error codes used in responses (tech spec §3 "Response — Error cases").
const (
	CodeInvalidLocation    = "invalid_location"
	CodeInvalidParameter   = "invalid_parameter"
	CodeTimeOutOfRange     = "time_out_of_range"
	CodeServiceUnavailable = "service_unavailable"
	CodeNotImplemented     = "not_implemented"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, ErrorBody{Error: code, Message: msg})
}


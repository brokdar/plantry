package handlers

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErrorParams is writeError with interpolation params for the message
// ("Couldn't load {{host}}").
func writeErrorParams(w http.ResponseWriter, status int, messageKey string, params map[string]string) {
	writeJSON(w, status, map[string]any{
		"message_key": messageKey,
		"status":      status,
		"params":      params,
	})
}

func writeError(w http.ResponseWriter, status int, messageKey string) {
	writeJSON(w, status, map[string]any{
		"message_key": messageKey,
		"status":      status,
	})
}

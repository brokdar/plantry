package handlers

import (
	"encoding/json"
	"net/http"
)

// Health answers GET /api/health with the status and the build version.
func Health(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": version})
	}
}

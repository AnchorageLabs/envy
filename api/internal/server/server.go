package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

const version = "dev"

// NewRouter builds the API HTTP router.
func NewRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/health", healthHandler)
	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}{
		OK:      true,
		Version: version,
	})
}

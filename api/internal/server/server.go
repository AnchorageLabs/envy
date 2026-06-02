package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const version = "dev"

// NewRouter builds the API HTTP router.
func NewRouter(pool *pgxpool.Pool) http.Handler {
	s := &Server{pool: pool}

	r := chi.NewRouter()
	r.Get("/health", s.healthHandler)
	return r
}

// Server owns API HTTP handlers and their dependencies.
type Server struct {
	pool *pgxpool.Pool
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	dbOK := false
	if s.pool != nil && s.pool.Ping(r.Context()) == nil {
		dbOK = true
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
		DBOK    bool   `json:"db_ok"`
	}{
		OK:      true,
		Version: version,
		DBOK:    dbOK,
	})
}

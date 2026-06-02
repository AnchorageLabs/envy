package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/AnchorageLabs/envy/api/internal/server/authcontext"
	"github.com/AnchorageLabs/envy/api/internal/server/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const version = "dev"
const bcryptCost = 10

// NewRouter builds the API HTTP router.
func NewRouter(pool *pgxpool.Pool) http.Handler {
	s := &Server{pool: pool}

	r := chi.NewRouter()
	r.Get("/health", s.healthHandler)
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", s.registerHandler)
		r.Post("/login", s.loginHandler)
		r.With(middleware.Auth(s.pool)).Post("/logout", s.logoutHandler)
	})
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(s.pool))
		r.Post("/projects", s.createProjectHandler)
		r.Get("/projects", s.listProjectsHandler)
		r.Get("/projects/{slug}", s.getProjectHandler)
		r.Patch("/projects/{slug}", s.updateProjectHandler)
		r.Delete("/projects/{slug}", s.deleteProjectHandler)
	})
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

	writeJSON(w, http.StatusOK, struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
		DBOK    bool   `json:"db_ok"`
	}{
		OK:      true,
		Version: version,
		DBOK:    dbOK,
	})
}

type registerRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

func (s *Server) registerHandler(w http.ResponseWriter, r *http.Request) {
	if s.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Name = strings.TrimSpace(req.Name)
	if req.Email == "" || req.Name == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email, name, and password are required")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	user := &User{}
	err = s.pool.QueryRow(r.Context(), `
		insert into users (email, name, password_hash)
		values ($1, $2, $3)
		returning id::text, email, name
	`, req.Email, req.Name, string(passwordHash)).Scan(&user.ID, &user.Email, &user.Name)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "user already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	if s.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user := &User{}
	var passwordHash string
	err := s.pool.QueryRow(r.Context(), `
		select id::text, email, name, password_hash
		from users
		where email = $1
	`, req.Email).Scan(&user.ID, &user.Email, &user.Name, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to login")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, err := newBearerToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}
	tokenHash := hashToken(token)

	_, err = s.pool.Exec(r.Context(), `
		insert into api_tokens (id, user_id, token_hash, label)
		values (gen_random_uuid(), $1::uuid, $2, 'cli')
	`, user.ID, tokenHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{Token: token, User: user})
}

func (s *Server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	if s.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	tokenHash, ok := authcontext.TokenHashFromCtx(r.Context())
	if !ok || tokenHash == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tag, err := s.pool.Exec(r.Context(), `
		update api_tokens
		set revoked_at = now()
		where token_hash = $1 and revoked_at is null
	`, tokenHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to logout")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	writeJSON(w, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func newBearerToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

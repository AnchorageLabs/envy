package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/AnchorageLabs/envy/api/internal/repo"
	"github.com/AnchorageLabs/envy/api/internal/server/authcontext"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

var projectSlugRE = regexp.MustCompile(`^[a-z0-9-]+$`)

type createProjectRequest struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type updateProjectRequest struct {
	Name *string `json:"name"`
}

func (s *Server) createProjectHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := s.requireAuthenticatedUserID(w, r)
	if !ok {
		return
	}

	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	req.Slug = strings.TrimSpace(req.Slug)
	req.Name = strings.TrimSpace(req.Name)
	if !isValidProjectSlug(req.Slug) {
		writeError(w, http.StatusBadRequest, "invalid project slug")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	project, err := repo.NewProjectStore(s.pool).CreateProject(r.Context(), ownerID, req.Slug, req.Name)
	if errors.Is(err, repo.ErrConflict) {
		writeError(w, http.StatusConflict, "project slug already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create project")
		return
	}

	writeJSON(w, http.StatusCreated, project)
}

func (s *Server) listProjectsHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := s.requireAuthenticatedUserID(w, r)
	if !ok {
		return
	}

	projects, err := repo.NewProjectStore(s.pool).ListProjectsByOwner(r.Context(), ownerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) getProjectHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := s.requireAuthenticatedUserID(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if !isValidProjectSlug(slug) {
		writeError(w, http.StatusBadRequest, "invalid project slug")
		return
	}

	project, err := repo.NewProjectStore(s.pool).GetProjectBySlugForOwner(r.Context(), ownerID, slug)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get project")
		return
	}

	writeJSON(w, http.StatusOK, project)
}

func (s *Server) updateProjectHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := s.requireAuthenticatedUserID(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if !isValidProjectSlug(slug) {
		writeError(w, http.StatusBadRequest, "invalid project slug")
		return
	}

	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	store := repo.NewProjectStore(s.pool)
	if req.Name == nil {
		project, err := store.GetProjectBySlugForOwner(r.Context(), ownerID, slug)
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get project")
			return
		}
		writeJSON(w, http.StatusOK, project)
		return
	}

	name := strings.TrimSpace(*req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	project, err := store.UpdateProjectNameForOwner(r.Context(), ownerID, slug, name)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update project")
		return
	}

	writeJSON(w, http.StatusOK, project)
}

func (s *Server) deleteProjectHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := s.requireAuthenticatedUserID(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if !isValidProjectSlug(slug) {
		writeError(w, http.StatusBadRequest, "invalid project slug")
		return
	}

	err := repo.NewProjectStore(s.pool).DeleteProjectForOwner(r.Context(), ownerID, slug)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete project")
		return
	}

	writeJSON(w, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (s *Server) requireAuthenticatedUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	if s.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return "", false
	}

	tokenHash, ok := authcontext.TokenHashFromCtx(r.Context())
	if !ok || tokenHash == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}

	var userID string
	err := s.pool.QueryRow(r.Context(), `
		select user_id::text
		from api_tokens
		where token_hash = $1 and revoked_at is null
	`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to authenticate")
		return "", false
	}

	return userID, true
}

func isValidProjectSlug(slug string) bool {
	return len(slug) >= 1 && len(slug) <= 60 && projectSlugRE.MatchString(slug)
}

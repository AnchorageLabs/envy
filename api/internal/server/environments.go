package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/AnchorageLabs/envy/api/internal/repo"
	"github.com/go-chi/chi/v5"
)

var environmentNameRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,30}$`)

type createEnvironmentRequest struct {
	Name string `json:"name"`
}

func (s *Server) registerEnvironmentRoutes(r chi.Router) {
	r.Route("/projects/{slug}/environments", func(r chi.Router) {
		r.Post("/", s.createEnvironmentHandler)
		r.Get("/", s.listEnvironmentsHandler)
		r.Get("/{name}", s.getEnvironmentHandler)
		r.Delete("/{name}", s.deleteEnvironmentHandler)
	})
}

func (s *Server) createEnvironmentHandler(w http.ResponseWriter, r *http.Request) {
	project, ok := s.projectForEnvironmentRequest(w, r)
	if !ok {
		return
	}

	var req createEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if !isValidEnvironmentName(req.Name) {
		writeError(w, http.StatusBadRequest, "invalid environment name")
		return
	}

	environment, err := repo.NewEnvironmentStore(s.pool).CreateEnvironment(r.Context(), project.ID, req.Name)
	if errors.Is(err, repo.ErrConflict) {
		writeError(w, http.StatusConflict, "environment already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create environment")
		return
	}

	writeJSON(w, http.StatusCreated, environment)
}

func (s *Server) listEnvironmentsHandler(w http.ResponseWriter, r *http.Request) {
	project, ok := s.projectForEnvironmentRequest(w, r)
	if !ok {
		return
	}

	environments, err := repo.NewEnvironmentStore(s.pool).ListEnvironmentsByProject(r.Context(), project.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list environments")
		return
	}

	writeJSON(w, http.StatusOK, environments)
}

func (s *Server) getEnvironmentHandler(w http.ResponseWriter, r *http.Request) {
	project, ok := s.projectForEnvironmentRequest(w, r)
	if !ok {
		return
	}

	name := chi.URLParam(r, "name")
	if !isValidEnvironmentName(name) {
		writeError(w, http.StatusBadRequest, "invalid environment name")
		return
	}

	environment, err := repo.NewEnvironmentStore(s.pool).GetEnvironmentByProjectAndName(r.Context(), project.ID, name)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "environment not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get environment")
		return
	}

	writeJSON(w, http.StatusOK, environment)
}

func (s *Server) deleteEnvironmentHandler(w http.ResponseWriter, r *http.Request) {
	project, ok := s.projectForEnvironmentRequest(w, r)
	if !ok {
		return
	}

	name := chi.URLParam(r, "name")
	if !isValidEnvironmentName(name) {
		writeError(w, http.StatusBadRequest, "invalid environment name")
		return
	}

	store := repo.NewEnvironmentStore(s.pool)
	environment, err := store.GetEnvironmentByProjectAndName(r.Context(), project.ID, name)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "environment not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get environment")
		return
	}
	if environment.StableVersionID != nil {
		writeError(w, http.StatusConflict, "environment has a stable version")
		return
	}

	err = store.DeleteEnvironmentByProjectAndName(r.Context(), project.ID, name)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "environment not found")
		return
	}
	if errors.Is(err, repo.ErrConflict) {
		writeError(w, http.StatusConflict, "environment has a stable version")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete environment")
		return
	}

	writeJSON(w, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (s *Server) projectForEnvironmentRequest(w http.ResponseWriter, r *http.Request) (*repo.Project, bool) {
	ownerID, ok := s.requireAuthenticatedUserID(w, r)
	if !ok {
		return nil, false
	}

	slug := chi.URLParam(r, "slug")
	if !isValidProjectSlug(slug) {
		writeError(w, http.StatusBadRequest, "invalid project slug")
		return nil, false
	}

	project, err := repo.NewProjectStore(s.pool).GetProjectBySlug(r.Context(), slug)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return nil, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get project")
		return nil, false
	}
	if project.OwnerID != ownerID {
		writeError(w, http.StatusForbidden, "forbidden")
		return nil, false
	}

	return project, true
}

func isValidEnvironmentName(name string) bool {
	return environmentNameRE.MatchString(name)
}

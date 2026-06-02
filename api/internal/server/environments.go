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
var schemaOwnerIDRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type createEnvironmentRequest struct {
	Name string `json:"name"`
}

type schemaVariableRequest struct {
	Key         string          `json:"key"`
	Type        string          `json:"type"`
	Required    bool            `json:"required"`
	Secret      bool            `json:"secret"`
	Default     *string         `json:"default"`
	Description string          `json:"description"`
	Owner       *string         `json:"owner"`
	Deprecated  bool            `json:"deprecated"`
	UpdatedAt   json.RawMessage `json:"updated_at,omitempty"`
}

func (s *Server) registerEnvironmentRoutes(r chi.Router) {
	r.Route("/projects/{slug}/environments", func(r chi.Router) {
		r.Post("/", s.createEnvironmentHandler)
		r.Get("/", s.listEnvironmentsHandler)
		r.Get("/{name}/schema", s.getEnvironmentSchemaHandler)
		r.Put("/{name}/schema", s.replaceEnvironmentSchemaHandler)
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

func (s *Server) getEnvironmentSchemaHandler(w http.ResponseWriter, r *http.Request) {
	project, environment, ok := s.projectAndEnvironmentForSchemaRequest(w, r)
	if !ok {
		return
	}

	_ = project
	schema, err := repo.NewVariableStore(s.pool).ListDraftSchema(r.Context(), environment.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get environment schema")
		return
	}

	writeJSON(w, http.StatusOK, schema)
}

func (s *Server) replaceEnvironmentSchemaHandler(w http.ResponseWriter, r *http.Request) {
	project, environment, ok := s.projectAndEnvironmentForSchemaRequest(w, r)
	if !ok {
		return
	}

	actorID, ok := s.requireAuthenticatedUserID(w, r)
	if !ok {
		return
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var req []schemaVariableRequest
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid schema")
		return
	}

	schema, ok := schemaVariablesFromRequest(w, req)
	if !ok {
		return
	}

	persisted, err := repo.NewVariableStore(s.pool).ReplaceDraftSchema(r.Context(), actorID, project.ID, environment.ID, schema)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to replace environment schema")
		return
	}

	writeJSON(w, http.StatusOK, persisted)
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

func (s *Server) projectAndEnvironmentForSchemaRequest(w http.ResponseWriter, r *http.Request) (*repo.Project, *repo.Environment, bool) {
	project, ok := s.projectForEnvironmentRequest(w, r)
	if !ok {
		return nil, nil, false
	}

	name := chi.URLParam(r, "name")
	if !isValidEnvironmentName(name) {
		writeError(w, http.StatusBadRequest, "invalid environment name")
		return nil, nil, false
	}

	environment, err := repo.NewEnvironmentStore(s.pool).GetEnvironmentByProjectAndName(r.Context(), project.ID, name)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "environment not found")
		return nil, nil, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get environment")
		return nil, nil, false
	}

	return project, environment, true
}

func schemaVariablesFromRequest(w http.ResponseWriter, req []schemaVariableRequest) ([]repo.VariableDefinition, bool) {
	seen := make(map[string]struct{}, len(req))
	schema := make([]repo.VariableDefinition, 0, len(req))

	for _, item := range req {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			writeError(w, http.StatusBadRequest, "variable key is required")
			return nil, false
		}
		if _, exists := seen[key]; exists {
			writeError(w, http.StatusBadRequest, "duplicate variable key")
			return nil, false
		}
		seen[key] = struct{}{}

		variableType := strings.TrimSpace(strings.ToLower(item.Type))
		if !repo.IsAllowedVariableType(variableType) {
			writeError(w, http.StatusBadRequest, "invalid variable type")
			return nil, false
		}

		if item.Owner != nil {
			owner := strings.TrimSpace(*item.Owner)
			item.Owner = &owner
			if owner == "" || !schemaOwnerIDRE.MatchString(owner) {
				writeError(w, http.StatusBadRequest, "invalid variable owner")
				return nil, false
			}
		}

		schema = append(schema, repo.VariableDefinition{
			Key:         key,
			Type:        variableType,
			Required:    item.Required,
			Secret:      item.Secret,
			Default:     item.Default,
			Description: item.Description,
			Owner:       item.Owner,
			Deprecated:  item.Deprecated,
		})
	}

	return schema, true
}

func isValidEnvironmentName(name string) bool {
	return environmentNameRE.MatchString(name)
}

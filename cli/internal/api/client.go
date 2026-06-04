package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Client is the shared ENVY API client used by CLI commands.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewClient constructs a Client. If httpClient is nil, http.DefaultClient is used.
func NewClient(baseURL string, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    httpClient,
	}
}

// Environment is the response shape for GET /environments/{name}.
//
// NOTE: Field names are based on the plan's stated assumptions
// (stable_version_id, version number). Confirm against api/ handlers.
type Environment struct {
	Name            string `json:"name"`
	StableVersionID string `json:"stable_version_id"`
	StableVersion   int    `json:"stable_version"`
}

// GetEnvironment fetches the environment metadata for the given project/environment.
func (c *Client) GetEnvironment(projectSlug string, environmentName string) (*Environment, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, fmt.Errorf("api url is required")
	}
	if strings.TrimSpace(projectSlug) == "" {
		return nil, fmt.Errorf("project slug is required")
	}
	if strings.TrimSpace(environmentName) == "" {
		return nil, fmt.Errorf("environment name is required")
	}

	baseURL, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid api url: %w", err)
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/projects/" + url.PathEscape(projectSlug) + "/environments/" + url.PathEscape(environmentName)

	req, err := http.NewRequest(http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("api request failed: %s", message)
	}

	var env Environment
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("failed to parse environment response: %w", err)
	}

	return &env, nil
}

// GetEnvironmentSchema fetches the draft schema for an environment.
func (c *Client) GetEnvironmentSchema(projectSlug string, environmentName string) (json.RawMessage, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, fmt.Errorf("api url is required")
	}
	if strings.TrimSpace(projectSlug) == "" {
		return nil, fmt.Errorf("project slug is required")
	}
	if strings.TrimSpace(environmentName) == "" {
		return nil, fmt.Errorf("environment name is required")
	}

	baseURL, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid api url: %w", err)
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/projects/" + url.PathEscape(projectSlug) + "/environments/" + url.PathEscape(environmentName) + "/schema"

	req, err := http.NewRequest(http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("api request failed: %s", message)
	}

	return json.RawMessage(body), nil
}

// ProposalChange is a single change operation in a proposal payload.
//
// NOTE: Field names (op/key/type/required/secret/value) are based on the
// plan's stated `changes` ops contract. Confirm against api/ proposal
// handlers. Pointer fields are omitted when nil so updates only send
// changed fields.
type ProposalChange struct {
	Op       string  `json:"op"`
	Key      string  `json:"key"`
	Type     string  `json:"type,omitempty"`
	Required *bool   `json:"required,omitempty"`
	Secret   *bool   `json:"secret,omitempty"`
	Value    *string `json:"value,omitempty"`
}

// proposalRequest is the request body for POST /proposals.
type proposalRequest struct {
	Message string           `json:"message"`
	Changes []ProposalChange `json:"changes"`
}

// Proposal is the response shape for a created proposal.
type Proposal struct {
	ID int `json:"id"`
}

// CreateProposal submits a proposal containing the given changes for the
// bound project/environment.
func (c *Client) CreateProposal(projectSlug string, environmentName string, message string, changes []ProposalChange) (*Proposal, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, fmt.Errorf("api url is required")
	}
	if strings.TrimSpace(projectSlug) == "" {
		return nil, fmt.Errorf("project slug is required")
	}
	if strings.TrimSpace(environmentName) == "" {
		return nil, fmt.Errorf("environment name is required")
	}
	if strings.TrimSpace(message) == "" {
		return nil, fmt.Errorf("proposal message is required")
	}

	baseURL, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid api url: %w", err)
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") +
		"/projects/" + url.PathEscape(projectSlug) +
		"/environments/" + url.PathEscape(environmentName) +
		"/proposals"

	reqBody := proposalRequest{Message: message, Changes: changes}
	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to encode proposal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL.String(), bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("api request failed: %s", message)
	}

	var proposal Proposal
	if err := json.Unmarshal(body, &proposal); err != nil {
		return nil, fmt.Errorf("failed to parse proposal response: %w", err)
	}

	return &proposal, nil
}

// ApproveProposalResult is the response shape for approving a proposal.
//
// NOTE: Field names are based on the plan's stated assumptions: the approve
// endpoint resolves a pending proposal, publishes a new stable version, and
// returns the affected environment name plus the new version number. We try
// several common field names to be resilient to the actual API shape; confirm
// against api/ proposal handlers.
type ApproveProposalResult struct {
	Environment string `json:"environment"`
	Env         string `json:"env"`
	EnvName     string `json:"env_name"`

	Version          int `json:"version"`
	PublishedVersion int `json:"published_version"`
	StableVersion    int `json:"stable_version"`
}

// EnvironmentName returns the affected environment name, tolerating multiple
// possible response field names.
func (r *ApproveProposalResult) EnvironmentName() string {
	if strings.TrimSpace(r.Environment) != "" {
		return r.Environment
	}
	if strings.TrimSpace(r.Env) != "" {
		return r.Env
	}
	return strings.TrimSpace(r.EnvName)
}

// PublishedVersionNumber returns the newly published version number,
// tolerating multiple possible response field names.
func (r *ApproveProposalResult) PublishedVersionNumber() int {
	if r.PublishedVersion != 0 {
		return r.PublishedVersion
	}
	if r.Version != 0 {
		return r.Version
	}
	return r.StableVersion
}

// ApproveProposal approves a pending proposal by ID. On success the API
// publishes a new stable version and returns the affected environment and the
// published version number.
//
// Errors from the API (e.g. attempting to approve an already-resolved or
// non-pending proposal) are surfaced with the server-provided message so
// callers can present a clear, user-facing error.
func (c *Client) ApproveProposal(proposalID string) (*ApproveProposalResult, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, fmt.Errorf("api url is required")
	}
	if strings.TrimSpace(proposalID) == "" {
		return nil, fmt.Errorf("proposal id is required")
	}

	baseURL, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid api url: %w", err)
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") +
		"/proposals/" + url.PathEscape(strings.TrimSpace(proposalID)) +
		"/approve"

	req, err := http.NewRequest(http.MethodPost, baseURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := decodeAPIErrorMessage(body)
		if message == "" {
			message = resp.Status
		}

		switch resp.StatusCode {
		case http.StatusConflict, http.StatusUnprocessableEntity, http.StatusBadRequest:
			// Typically returned when the proposal is not pending / already
			// resolved. Surface a clear, user-facing message.
			return nil, fmt.Errorf("cannot approve proposal: %s", message)
		case http.StatusNotFound:
			return nil, fmt.Errorf("proposal not found: %s", message)
		default:
			return nil, fmt.Errorf("api request failed: %s", message)
		}
	}

	var result ApproveProposalResult
	if len(body) > 0 {
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse approve response: %w", err)
		}
	}

	return &result, nil
}

// decodeAPIErrorMessage extracts a human-readable error message from a JSON
// error body, falling back to the raw body text. Supports common shapes like
// `{"error": "..."}` and `{"message": "..."}`.
func decodeAPIErrorMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}

	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if strings.TrimSpace(payload.Error) != "" {
			return strings.TrimSpace(payload.Error)
		}
		if strings.TrimSpace(payload.Message) != "" {
			return strings.TrimSpace(payload.Message)
		}
	}

	return trimmed
}

// versionValuesResponse is the response shape for the version values endpoint.
//
// NOTE: The endpoint may return either a flat map of key->value or an object
// with a `values` field. We support both to be resilient to the actual API
// shape; confirm against api/ handlers.
type versionValuesResponse struct {
	Values map[string]string `json:"values"`
}

// GetEnvironmentVersionValues fetches the values for a specific published
// version of an environment.
func (c *Client) GetEnvironmentVersionValues(projectSlug string, environmentName string, version int) (map[string]string, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, fmt.Errorf("api url is required")
	}
	if strings.TrimSpace(projectSlug) == "" {
		return nil, fmt.Errorf("project slug is required")
	}
	if strings.TrimSpace(environmentName) == "" {
		return nil, fmt.Errorf("environment name is required")
	}

	baseURL, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid api url: %w", err)
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") +
		"/projects/" + url.PathEscape(projectSlug) +
		"/environments/" + url.PathEscape(environmentName) +
		"/versions/" + url.PathEscape(strconv.Itoa(version)) +
		"/values"

	req, err := http.NewRequest(http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("api request failed: %s", message)
	}

	// Try the wrapped `{ "values": {...} }` shape first.
	var wrapped versionValuesResponse
	if err := json.Unmarshal(body, &wrapped); err == nil && wrapped.Values != nil {
		return wrapped.Values, nil
	}

	// Fall back to a flat key->value map.
	var flat map[string]string
	if err := json.Unmarshal(body, &flat); err != nil {
		return nil, fmt.Errorf("failed to parse version values response: %w", err)
	}
	if flat == nil {
		flat = map[string]string{}
	}
	return flat, nil
}

// RollbackEnvironment re-points an environment's stable version to a previously
// published version.
//
// NOTE: not yet wired end to end. The repo layer already implements the
// operation (api/internal/repo/environment_versions.go:RollbackEnvironmentToVersion),
// but no HTTP route exposes it and this client lacks the project-slug context the
// rest of the API takes. Returning a clear error keeps `envy rollback` honest
// (and the CLI buildable) until the server endpoint and project plumbing land.
func (c *Client) RollbackEnvironment(environmentName string, version int) error {
	return fmt.Errorf(
		"rollback is not yet available: the API rollback endpoint is not wired up "+
			"(repo store exists, HTTP route and project context missing) — %s@%d",
		environmentName, version,
	)
}

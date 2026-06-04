package api

import (
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

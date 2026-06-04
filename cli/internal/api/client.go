package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

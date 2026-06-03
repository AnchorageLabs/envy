package api

import "net/http"

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

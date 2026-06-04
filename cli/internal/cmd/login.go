package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var errCredentialsNotFound = errors.New("credentials not found")

type loginOptions struct {
	email string
}

type apiLoginResponse struct {
	Token       string        `json:"token"`
	AccessToken string        `json:"access_token"`
	JWT         string        `json:"jwt"`
	Name        string        `json:"name"`
	Email       string        `json:"email"`
	User        *apiLoginUser `json:"user"`
	Data        *apiLoginData `json:"data"`
}

type apiLoginData struct {
	Token       string        `json:"token"`
	AccessToken string        `json:"access_token"`
	JWT         string        `json:"jwt"`
	Name        string        `json:"name"`
	Email       string        `json:"email"`
	User        *apiLoginUser `json:"user"`
}

type apiLoginUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func newLoginCommand(rootOpts *rootOptions) *cobra.Command {
	opts := &loginOptions{}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with the ENVY API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL := canonicalAPIURL(rootOpts.resolvedAPIURL)
			if apiURL == "" {
				return errors.New("no API URL configured; pass --api-url, set ENVY_API_URL, or run envy init in a configured project")
			}

			email := strings.TrimSpace(opts.email)
			if email == "" {
				promptedEmail, err := promptEmail(cmd)
				if err != nil {
					return err
				}
				email = promptedEmail
			}
			if email == "" {
				return errors.New("email is required")
			}

			password, err := promptPassword(cmd)
			if err != nil {
				return err
			}
			if password == "" {
				return errors.New("password is required")
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			loginResponse, err := loginToAPI(ctx, apiURL, email, password)
			if err != nil {
				return err
			}

			token := loginResponse.authToken()
			if token == "" {
				return errors.New("login succeeded but the API response did not include an authentication token")
			}

			if err := saveCredential(apiURL, token); err != nil {
				return err
			}
			rootOpts.apiToken = token

			name, responseEmail := loginResponse.identity()
			if responseEmail == "" {
				responseEmail = email
			}
			if name == "" {
				name = responseEmail
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s (%s)\n", name, responseEmail)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.email, "email", "", "email address")

	return cmd
}

func promptEmail(cmd *cobra.Command) (string, error) {
	fmt.Fprint(cmd.OutOrStdout(), "Email: ")

	reader := bufio.NewReader(cmd.InOrStdin())
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	return strings.TrimSpace(line), nil
}

func promptPassword(cmd *cobra.Command) (string, error) {
	inputFile, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return "", errors.New("password prompt requires an interactive terminal")
	}

	fd := int(inputFile.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("password prompt requires an interactive terminal")
	}

	fmt.Fprint(cmd.OutOrStdout(), "Password: ")
	passwordBytes, err := term.ReadPassword(fd)
	fmt.Fprintln(cmd.OutOrStdout())
	if err != nil {
		return "", err
	}

	return string(passwordBytes), nil
}

func loginToAPI(ctx context.Context, apiURL, email, password string) (apiLoginResponse, error) {
	var loginResponse apiLoginResponse

	requestBody, err := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	if err != nil {
		return loginResponse, err
	}

	endpoint := strings.TrimRight(apiURL, "/") + "/auth/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return loginResponse, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return loginResponse, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return loginResponse, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := responseErrorMessage(responseBody)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			if message == "" {
				message = "invalid email or password"
			}
			return loginResponse, fmt.Errorf("authentication failed: %s", message)
		}
		if message == "" {
			message = resp.Status
		}
		return loginResponse, fmt.Errorf("login failed: %s", message)
	}

	if len(strings.TrimSpace(string(responseBody))) == 0 {
		return loginResponse, errors.New("login succeeded but the API returned an empty response")
	}

	if err := json.Unmarshal(responseBody, &loginResponse); err != nil {
		return loginResponse, fmt.Errorf("failed to parse login response: %w", err)
	}

	return loginResponse, nil
}

func responseErrorMessage(responseBody []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return strings.TrimSpace(string(responseBody))
	}

	for _, key := range []string{"message", "error", "detail"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typedValue := value.(type) {
		case string:
			return typedValue
		case map[string]any:
			if message, ok := typedValue["message"].(string); ok {
				return message
			}
		}
	}

	return ""
}

func (r apiLoginResponse) authToken() string {
	if r.Token != "" {
		return r.Token
	}
	if r.AccessToken != "" {
		return r.AccessToken
	}
	if r.JWT != "" {
		return r.JWT
	}
	if r.Data != nil {
		if r.Data.Token != "" {
			return r.Data.Token
		}
		if r.Data.AccessToken != "" {
			return r.Data.AccessToken
		}
		if r.Data.JWT != "" {
			return r.Data.JWT
		}
	}
	return ""
}

func (r apiLoginResponse) identity() (string, string) {
	name := r.Name
	email := r.Email

	if r.User != nil {
		if name == "" {
			name = r.User.Name
		}
		if email == "" {
			email = r.User.Email
		}
	}

	if r.Data != nil {
		if name == "" {
			name = r.Data.Name
		}
		if email == "" {
			email = r.Data.Email
		}
		if r.Data.User != nil {
			if name == "" {
				name = r.Data.User.Name
			}
			if email == "" {
				email = r.Data.User.Email
			}
		}
	}

	return name, email
}

func loadCredential(apiURL string) (string, error) {
	credentials, err := readCredentialStore()
	if err != nil {
		return "", err
	}

	apiURL = canonicalAPIURL(apiURL)
	if token := credentials[apiURL]; token != "" {
		return token, nil
	}

	// Backward compatibility with an old single-token credentials file.
	if token := credentials[""]; token != "" {
		return token, nil
	}

	return "", errCredentialsNotFound
}

func saveCredential(apiURL, token string) error {
	credentials, err := readCredentialStore()
	if err != nil && !errors.Is(err, errCredentialsNotFound) {
		return err
	}
	if credentials == nil {
		credentials = make(map[string]string)
	}

	credentials[canonicalAPIURL(apiURL)] = token
	delete(credentials, "")

	return writeCredentialStore(credentials)
}

func readCredentialStore() (map[string]string, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, err
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return map[string]string{}, nil
	}

	credentials := make(map[string]string)
	if err := json.Unmarshal(trimmed, &credentials); err == nil {
		return credentials, nil
	}

	var singleToken string
	if err := json.Unmarshal(trimmed, &singleToken); err == nil && singleToken != "" {
		return map[string]string{"": singleToken}, nil
	}

	plainToken := strings.TrimSpace(string(trimmed))
	if plainToken != "" {
		return map[string]string{"": plainToken}, nil
	}

	return nil, errors.New("credentials file is not valid JSON")
}

func writeCredentialStore(credentials map[string]string) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".credentials-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}

	return nil
}

func credentialsPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if homeDir == "" {
		return "", errors.New("could not determine home directory")
	}
	return filepath.Join(homeDir, ".envy", "credentials"), nil
}

func canonicalAPIURL(apiURL string) string {
	return strings.TrimRight(strings.TrimSpace(apiURL), "/")
}

func (opts *rootOptions) applyAuth(req *http.Request) {
	if opts == nil || opts.apiToken == "" || req == nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+opts.apiToken)
}

package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	envyDirPath       = ".envy"
	envyConfigPath    = ".envy/config.json"
	envySchemaPath    = ".envy/schema.json"
	envyLocalPath     = ".env.local"
	gitignoreFilePath = ".gitignore"
)

type initOptions struct {
	apiURL      string
	project     string
	environment string
}

type initProjectConfig struct {
	APIURL      string `json:"api_url"`
	Project     string `json:"project"`
	Environment string `json:"environment"`
}

type initAlreadyExistsError struct {
	path string
}

func (e initAlreadyExistsError) Error() string {
	return fmt.Sprintf("envy is already initialized: %s exists", e.path)
}

func (e initAlreadyExistsError) ExitCode() int {
	return 2
}

type gitignoreStatus string

const (
	gitignoreAbsent  gitignoreStatus = "absent"
	gitignoreUpdated gitignoreStatus = "updated"
	gitignoreAlready gitignoreStatus = "already"
)

func newInitCommand(rootOpts *rootOptions) *cobra.Command {
	opts := &initOptions{}

	cmd := &cobra.Command{
		Use:   "init [--api-url <url>] [--project <slug>] [--environment <name>]",
		Short: "Scaffold ENVY files in the current repository",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if rootOpts != nil {
				opts.apiURL = rootOpts.apiURL
				if opts.apiURL == "" {
					opts.apiURL = rootOpts.resolvedAPIURL
				}
			}
			if opts.apiURL == "" {
				opts.apiURL = os.Getenv("ENVY_API_URL")
			}

			return runInit(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.project, "project", "", "ENVY project slug")
	cmd.Flags().StringVar(&opts.environment, "environment", "", "default ENVY environment name")

	return cmd
}

func runInit(cmd *cobra.Command, opts *initOptions) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	configPath := filepath.Join(cwd, envyConfigPath)
	if exists, err := pathExists(configPath); err != nil {
		return err
	} else if exists {
		return initAlreadyExistsError{path: envyConfigPath}
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	if opts.apiURL == "" {
		opts.apiURL, err = promptRequired(reader, out, "API URL")
		if err != nil {
			return err
		}
	}
	if opts.project == "" {
		opts.project, err = promptRequired(reader, out, "Project")
		if err != nil {
			return err
		}
	}
	if opts.environment == "" {
		opts.environment, err = promptRequired(reader, out, "Environment")
		if err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Join(cwd, envyDirPath), 0755); err != nil {
		return err
	}

	created := []string{}
	unchanged := []string{}

	configBytes, err := json.MarshalIndent(initProjectConfig{
		APIURL:      opts.apiURL,
		Project:     opts.project,
		Environment: opts.environment,
	}, "", "  ")
	if err != nil {
		return err
	}
	configBytes = append(configBytes, '\n')

	if err := writeNewFile(configPath, configBytes, 0644); err != nil {
		if errors.Is(err, os.ErrExist) {
			return initAlreadyExistsError{path: envyConfigPath}
		}
		return err
	}
	created = append(created, envyConfigPath)

	schemaCreated, err := createFileIfMissing(filepath.Join(cwd, envySchemaPath), []byte("[]\n"), 0644)
	if err != nil {
		return err
	}
	if schemaCreated {
		created = append(created, envySchemaPath)
	} else {
		unchanged = append(unchanged, envySchemaPath)
	}

	envLocalCreated, err := createFileIfMissing(filepath.Join(cwd, envyLocalPath), []byte("# Local development values hydrated by `envy pull`\n"), 0600)
	if err != nil {
		return err
	}
	if envLocalCreated {
		created = append(created, envyLocalPath)
	} else {
		unchanged = append(unchanged, envyLocalPath)
	}

	gitignore, err := updateGitignore(cwd)
	if err != nil {
		return err
	}

	printInitSummary(out, created, unchanged, gitignore)
	return nil
}

func promptRequired(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	fmt.Fprintf(out, "%s: ", label)

	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}

	return value, nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func writeNewFile(path string, data []byte, perm os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}

	return file.Close()
}

func createFileIfMissing(path string, data []byte, perm os.FileMode) (bool, error) {
	if err := writeNewFile(path, data, perm); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func updateGitignore(cwd string) (gitignoreStatus, error) {
	path := filepath.Join(cwd, gitignoreFilePath)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return gitignoreAbsent, nil
		}
		return "", err
	}

	if gitignoreContainsEnvLocal(string(data)) {
		return gitignoreAlready, nil
	}

	updated := string(data)
	if updated != "" && !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	updated += envyLocalPath + "\n"

	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return "", err
	}

	return gitignoreUpdated, nil
}

func gitignoreContainsEnvLocal(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == envyLocalPath {
			return true
		}
	}

	return false
}

func printInitSummary(out io.Writer, created []string, unchanged []string, gitignore gitignoreStatus) {
	fmt.Fprintln(out, "ENVY initialized successfully.")
	fmt.Fprintln(out)

	fmt.Fprintln(out, "Created files:")
	for _, path := range created {
		fmt.Fprintf(out, "  - %s\n", path)
	}

	if len(unchanged) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Left unchanged:")
		for _, path := range unchanged {
			fmt.Fprintf(out, "  - %s\n", path)
		}
	}

	fmt.Fprintln(out)
	switch gitignore {
	case gitignoreUpdated:
		fmt.Fprintf(out, "Updated %s with %s.\n", gitignoreFilePath, envyLocalPath)
	case gitignoreAlready:
		fmt.Fprintf(out, "%s already contains %s.\n", gitignoreFilePath, envyLocalPath)
	case gitignoreAbsent:
		fmt.Fprintf(out, "%s not found; left unchanged.\n", gitignoreFilePath)
	}
}

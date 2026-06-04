package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AnchorageLabs/envy/cli/internal/api"
	"github.com/AnchorageLabs/envy/cli/internal/config"
	"github.com/spf13/cobra"
)

type exportOptions struct {
	root *rootOptions
	out  string
}

// exportExitError signals the CLI to exit with a specific code while still
// surfacing a clear message to stderr (handled by Execute via ExitCode()).
type exportExitError struct {
	msg  string
	code int
}

func (e *exportExitError) Error() string { return e.msg }
func (e *exportExitError) ExitCode() int { return e.code }

func newExportCommand(root *rootOptions) *cobra.Command {
	opts := &exportOptions{root: root}

	cmd := &cobra.Command{
		Use:   "export [env]",
		Short: "Rewrite .env.local (or stdout) from the synced lockfile state",
		Long:  "Load .envy/lock.json, fetch the decrypted values for the locked version via the API, and write them as sorted KEY=VALUE pairs to .env.local (default) or stdout when --out is '-'.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd, opts, args)
		},
	}

	cmd.Flags().StringVar(&opts.out, "out", ".env.local", "output file path, or '-' for stdout")

	return cmd
}

func runExport(cmd *cobra.Command, opts *exportOptions, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	projectConfig, err := config.LoadProjectConfig(cwd)
	if err != nil {
		if errors.Is(err, config.ErrProjectConfigNotFound) {
			return fmt.Errorf("no ENVY project found: run `envy init` first")
		}
		return err
	}

	lockPath := filepath.Join(cwd, ".envy", "lock.json")
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(cmd.ErrOrStderr(), "error: .envy/lock.json not found: run `envy pull` first")
			return &exportExitError{msg: ".envy/lock.json not found", code: 1}
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to read .envy/lock.json: %v\n", err)
		return &exportExitError{msg: err.Error(), code: 1}
	}

	var lock lockFile
	if err := json.Unmarshal(lockData, &lock); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to parse .envy/lock.json: %v\n", err)
		return &exportExitError{msg: err.Error(), code: 1}
	}

	envName := ""
	if len(args) == 1 {
		envName = strings.TrimSpace(args[0])
	}
	if envName == "" {
		envName = strings.TrimSpace(projectConfig.Environment)
	}
	if envName == "" {
		// Fall back to the sole environment in the lockfile if unambiguous.
		if len(lock.Environments) == 1 {
			for name := range lock.Environments {
				envName = name
			}
		}
	}
	if envName == "" {
		return fmt.Errorf("no environment specified: pass one as an argument or set a default environment with `envy init`")
	}

	block, ok := lock.Environments[envName]
	if !ok {
		return fmt.Errorf("environment %q not found in .envy/lock.json: run `envy pull %s` first", envName, envName)
	}

	project := projectConfig.Project
	if project == "" {
		project = lock.Project
	}

	if strings.TrimSpace(opts.root.resolvedAPIURL) == "" {
		return fmt.Errorf("api url is required: configure it in .envy/config.json, set ENVY_API_URL, or pass --api-url")
	}

	client := api.NewClient(opts.root.resolvedAPIURL, opts.root.apiToken, nil)

	values, err := client.GetEnvironmentVersionValues(project, envName, block.Version)
	if err != nil {
		return err
	}

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if opts.out == "-" {
		var b strings.Builder
		for _, key := range keys {
			b.WriteString(key)
			b.WriteString("=")
			b.WriteString(values[key])
			b.WriteString("\n")
		}
		fmt.Fprint(cmd.OutOrStdout(), b.String())
		return nil
	}

	outPath := opts.out
	if !filepath.IsAbs(outPath) {
		outPath = filepath.Join(cwd, outPath)
	}

	if err := writeEnvLocal(outPath, keys, values); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Exported %d keys from %s@%d to %s\n", len(keys), envName, block.Version, opts.out)
	return nil
}

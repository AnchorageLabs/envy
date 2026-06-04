package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/AnchorageLabs/envy/cli/internal/api"
	"github.com/AnchorageLabs/envy/cli/internal/config"
	"github.com/AnchorageLabs/envy/cli/internal/schema"
	"github.com/spf13/cobra"
)

// exitError carries a process exit code so the root command can propagate it.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit code %d", e.code)
	}
	return e.err.Error()
}

func (e *exitError) ExitCode() int { return e.code }

func (e *exitError) Unwrap() error { return e.err }

type verifyOptions struct {
	root    *rootOptions
	env     string
	against string
	json    bool
}

// verifyFailure describes a single failed check for a variable.
type verifyFailure struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

// verifyReport is the structured result of a verification run.
type verifyReport struct {
	Environment    string          `json:"environment"`
	Version        int             `json:"version"`
	Against        string          `json:"against"`
	Status         string          `json:"status"`
	Missing        []verifyFailure `json:"missing"`
	TypeMismatches []verifyFailure `json:"type_mismatches"`
	Deprecatedused []verifyFailure `json:"deprecated_in_use"`
}

func newVerifyCommand(root *rootOptions) *cobra.Command {
	opts := &verifyOptions{root: root, against: "stable"}

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Validate environment health as a CI gate",
		Long:  "Verify that required variables are present, values match their declared types, and no deprecated keys are still in use. This command never writes files.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.env, "env", "", "environment name to verify")
	cmd.Flags().StringVar(&opts.against, "against", "stable", "version pointer to verify against")
	cmd.Flags().BoolVar(&opts.json, "json", false, "emit a structured JSON report to stdout")

	return cmd
}

func runVerify(cmd *cobra.Command, opts *verifyOptions) error {
	cwd, err := os.Getwd()
	if err != nil {
		return &exitError{code: 2, err: err}
	}

	projectConfig, cfgErr := config.LoadProjectConfig(cwd)

	envName := strings.TrimSpace(opts.env)
	if envName == "" && cfgErr == nil {
		envName = strings.TrimSpace(projectConfig.Environment)
	}
	if envName == "" {
		return &exitError{code: 2, err: fmt.Errorf("environment is required: pass --env <name>")}
	}

	projectSlug := ""
	if cfgErr == nil {
		projectSlug = strings.TrimSpace(projectConfig.Project)
	}
	if projectSlug == "" {
		return &exitError{code: 2, err: fmt.Errorf("project slug is required: ensure .envy/config.json defines a project")}
	}

	if strings.TrimSpace(opts.root.resolvedAPIURL) == "" {
		return &exitError{code: 2, err: fmt.Errorf("api url is required")}
	}

	client := api.NewClient(opts.root.resolvedAPIURL, opts.root.apiToken, nil)

	// Resolve target version via the environment's stable pointer.
	environment, err := client.GetEnvironment(projectSlug, envName)
	if err != nil {
		return &exitError{code: 2, err: fmt.Errorf("failed to fetch environment: %w", err)}
	}

	version := environment.StableVersion
	if version <= 0 {
		return &exitError{code: 2, err: fmt.Errorf("environment %q has no resolvable %s version", envName, opts.against)}
	}

	versionValues, err := client.GetEnvironmentVersionValues(projectSlug, envName, version)
	if err != nil {
		return &exitError{code: 2, err: fmt.Errorf("failed to fetch version values: %w", err)}
	}

	sch, err := schema.Load("")
	if err != nil {
		return &exitError{code: 2, err: fmt.Errorf("failed to load schema: %w", err)}
	}

	// .env.local is optional; a missing file is non-fatal.
	localValues, err := loadEnvLocal(".env.local")
	if err != nil {
		return &exitError{code: 2, err: fmt.Errorf("failed to read .env.local: %w", err)}
	}

	// Merge version values with .env.local overrides.
	effective := make(map[string]string, len(versionValues)+len(localValues))
	for k, v := range versionValues {
		effective[k] = v
	}
	for k, v := range localValues {
		effective[k] = v
	}

	report := buildReport(envName, version, opts.against, sch, effective)

	if opts.json {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return &exitError{code: 2, err: err}
		}
	} else {
		printHumanReport(cmd, report)
	}

	if report.Status != "healthy" {
		return &exitError{code: 1}
	}

	return nil
}

func buildReport(envName string, version int, against string, sch *schema.Schema, effective map[string]string) verifyReport {
	report := verifyReport{
		Environment:    envName,
		Version:        version,
		Against:        against,
		Missing:        []verifyFailure{},
		TypeMismatches: []verifyFailure{},
		Deprecatedused: []verifyFailure{},
	}

	for _, field := range sch.Fields {
		value, present := effective[field.Name]
		hasValue := present && strings.TrimSpace(value) != ""

		if field.Required && !hasValue {
			report.Missing = append(report.Missing, verifyFailure{
				Key:    field.Name,
				Reason: "required variable has no value",
			})
			continue
		}

		if hasValue {
			if err := field.ValidateValue(value); err != nil {
				report.TypeMismatches = append(report.TypeMismatches, verifyFailure{
					Key:    field.Name,
					Reason: err.Error(),
				})
			}

			if field.Deprecated {
				report.Deprecatedused = append(report.Deprecatedused, verifyFailure{
					Key:    field.Name,
					Reason: "deprecated variable is still in use",
				})
			}
		}
	}

	if len(report.Missing) == 0 && len(report.TypeMismatches) == 0 && len(report.Deprecatedused) == 0 {
		report.Status = "healthy"
	} else {
		report.Status = "failed"
	}

	return report
}

func printHumanReport(cmd *cobra.Command, report verifyReport) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Verifying %s@%d (%s)\n", report.Environment, report.Version, report.Against)

	if report.Status == "healthy" {
		fmt.Fprintln(out, "Status: healthy — all required variables present, types valid, no deprecated keys in use.")
		return
	}

	fmt.Fprintln(out, "Status: failed")

	if len(report.Missing) > 0 {
		fmt.Fprintln(out, "\nMissing required variables:")
		for _, f := range report.Missing {
			fmt.Fprintf(out, "  - %s: %s\n", f.Key, f.Reason)
		}
	}

	if len(report.TypeMismatches) > 0 {
		fmt.Fprintln(out, "\nType mismatches:")
		for _, f := range report.TypeMismatches {
			fmt.Fprintf(out, "  - %s: %s\n", f.Key, f.Reason)
		}
	}

	if len(report.Deprecatedused) > 0 {
		fmt.Fprintln(out, "\nDeprecated keys in use:")
		for _, f := range report.Deprecatedused {
			fmt.Fprintf(out, "  - %s: %s\n", f.Key, f.Reason)
		}
	}
}

// loadEnvLocal reads a dotenv-style file into a key->value map. A missing file
// returns an empty map and no error.
func loadEnvLocal(path string) (map[string]string, error) {
	values := map[string]string{}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return values, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}

		key := strings.TrimSpace(line[:eq])
		value := strings.TrimSpace(line[eq+1:])
		if key == "" {
			continue
		}

		value = strings.Trim(value, "\"'")
		values[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Keep output deterministic for any future iteration (no-op for maps but
	// guards against accidental reliance on insertion order elsewhere).
	_ = sortedKeys(values)

	return values, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

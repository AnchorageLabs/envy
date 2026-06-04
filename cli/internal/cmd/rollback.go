package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AnchorageLabs/envy/cli/internal/api"
	"github.com/AnchorageLabs/envy/cli/internal/config"
	"github.com/spf13/cobra"
)

type rollbackOptions struct {
	root   *rootOptions
	noPull bool
}

func newRollbackCommand(root *rootOptions) *cobra.Command {
	opts := &rollbackOptions{root: root}

	cmd := &cobra.Command{
		Use:   "rollback <env>@<n>",
		Short: "Re-point an environment's stable version to a previous version",
		Long:  "Re-point an environment's stable version to a previously published version via the API. By default this also pulls to refresh the lockfile and .env.local; pass --no-pull to skip that step.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return runRollback(cmd, opts, args)
		},
	}

	cmd.Flags().BoolVar(&opts.noPull, "no-pull", false, "skip the implicit pull that refreshes the lockfile and .env.local")

	return cmd
}

func runRollback(cmd *cobra.Command, opts *rollbackOptions, args []string) error {
	envName, version, err := parseRollbackArg(args[0])
	if err != nil {
		return err
	}

	if strings.TrimSpace(opts.root.resolvedAPIURL) == "" {
		return fmt.Errorf("api url is required: configure it in .envy/config.json, set ENVY_API_URL, or pass --api-url")
	}

	client := api.NewClient(opts.root.resolvedAPIURL, opts.root.apiToken, nil)

	if err := client.RollbackEnvironment(envName, version); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Rolled %s back to %s@%d.\n", envName, envName, version)

	if opts.noPull {
		return nil
	}

	// Reuse the existing pull routine to refresh the lockfile and .env.local
	// for the affected environment.
	pullOpts := &pullOptions{root: opts.root}
	if err := runPull(cmd, pullOpts, []string{envName}); err != nil {
		return fmt.Errorf("rolled back, but failed to pull updated values: %w", err)
	}

	return nil
}

// parseRollbackArg splits an `<env>@<n>` argument, validating that the env name
// is non-empty and the version is a positive integer.
func parseRollbackArg(arg string) (string, int, error) {
	arg = strings.TrimSpace(arg)
	idx := strings.LastIndex(arg, "@")
	if idx < 0 {
		return "", 0, fmt.Errorf("invalid argument %q: expected format <env>@<n> (e.g. development@42)", arg)
	}

	envName := strings.TrimSpace(arg[:idx])
	versionStr := strings.TrimSpace(arg[idx+1:])

	if envName == "" {
		return "", 0, fmt.Errorf("invalid argument %q: environment name is required (expected <env>@<n>)", arg)
	}
	if versionStr == "" {
		return "", 0, fmt.Errorf("invalid argument %q: version is required (expected <env>@<n>)", arg)
	}

	version, err := strconv.Atoi(versionStr)
	if err != nil || version <= 0 {
		return "", 0, fmt.Errorf("invalid version %q: must be a positive integer", versionStr)
	}

	return envName, version, nil
}

// ensure config import is retained for parity with other commands that resolve
// project context; rollback relies on the API client and pull for project
// resolution, so config is referenced indirectly via runPull.
var _ = config.ErrProjectConfigNotFound

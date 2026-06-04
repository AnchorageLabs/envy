package cmd

import (
	"fmt"
	"strings"

	"github.com/AnchorageLabs/envy/cli/internal/api"
	"github.com/spf13/cobra"
)

type approveOptions struct {
	root   *rootOptions
	noPull bool
}

func newApproveCommand(root *rootOptions) *cobra.Command {
	opts := &approveOptions{root: root}

	cmd := &cobra.Command{
		Use:   "approve <proposalId>",
		Short: "Approve a pending proposal and publish a new stable version",
		Long:  "Approve a pending proposal via the API, publishing a new stable version for its environment. By default this also pulls to refresh the lockfile and .env.local; pass --no-pull to skip that step.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return runApprove(cmd, opts, args)
		},
	}

	cmd.Flags().BoolVar(&opts.noPull, "no-pull", false, "skip the implicit pull that refreshes the lockfile and .env.local")

	return cmd
}

func runApprove(cmd *cobra.Command, opts *approveOptions, args []string) error {
	proposalID := strings.TrimSpace(args[0])
	if proposalID == "" {
		return fmt.Errorf("proposal id is required")
	}

	if strings.TrimSpace(opts.root.resolvedAPIURL) == "" {
		return fmt.Errorf("api url is required: configure it in .envy/config.json, set ENVY_API_URL, or pass --api-url")
	}

	client := api.NewClient(opts.root.resolvedAPIURL, opts.root.apiToken, nil)

	result, err := client.ApproveProposal(proposalID)
	if err != nil {
		return err
	}

	envName := result.EnvironmentName()
	version := result.PublishedVersionNumber()

	fmt.Fprintf(cmd.OutOrStdout(), "Approved. Published %s@%d\n", envName, version)

	if opts.noPull {
		return nil
	}

	// Reuse the existing pull routine to refresh the lockfile and .env.local
	// for the affected environment. If the approve response did not include an
	// environment name, fall back to the project's default environment by
	// passing no args to pull.
	pullArgs := []string{}
	if strings.TrimSpace(envName) != "" {
		pullArgs = append(pullArgs, envName)
	}

	pullOpts := &pullOptions{root: opts.root}
	if err := runPull(cmd, pullOpts, pullArgs); err != nil {
		return fmt.Errorf("approved, but failed to pull updated values: %w", err)
	}

	return nil
}

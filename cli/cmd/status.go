package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"envy/cli/internal/api"
	"envy/cli/internal/lockfile"
)

var statusCmd = &cobra.Command{
	Use:           "status",
	Short:         "Show sync status of the local environment against the API",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatus(cmd, args)
	},
}

func runStatus(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	lf, err := lockfile.Load("")
	if err != nil {
		if errors.Is(err, lockfile.ErrNotFound) {
			fmt.Fprintln(out, "Not synced yet. Run: envy pull <env>")
			return nil
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
		return newExitError(2, err)
	}

	baseURL := os.Getenv("ENVY_API_URL")
	token := os.Getenv("ENVY_TOKEN")
	client := api.NewClient(baseURL, token, http.DefaultClient)

	env, err := client.GetEnvironment(lf.Project, lf.Environment)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
		return newExitError(2, err)
	}

	local := lf.Version
	stable := env.StableVersion

	switch {
	case local == stable:
		fmt.Fprintln(out, "Up to date.")
	case local < stable:
		n := stable - local
		fmt.Fprintf(out, "Your environment is behind by %d version(s). Run: envy pull\n", n)
	default:
		n := local - stable
		fmt.Fprintf(out, "Your environment is ahead by %d version(s).\n", n)
	}

	return nil
}

package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/AnchorageLabs/envy/cli/internal/config"
	"github.com/spf13/cobra"
)

// Version is set at build time by release builds. Development builds default to dev.
var Version = "dev"

type rootOptions struct {
	apiURL         string
	resolvedAPIURL string
	apiToken       string
	showVersion    bool
}

// Execute runs the ENVY root command.
func Execute() error {
	err := NewRootCommand().Execute()
	if err == nil {
		return nil
	}

	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}

	return err
}

// NewRootCommand constructs the ENVY root command.
func NewRootCommand() *cobra.Command {
	opts := &rootOptions{}

	rootCmd := &cobra.Command{
		Use:   "envy",
		Short: "Version control for environment variables",
		Long:  "ENVY keeps environment variable schemas and values synced, reviewed, validated, and recoverable.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.showVersion {
				fmt.Fprintln(cmd.OutOrStdout(), Version)
				return nil
			}

			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.showVersion {
				return nil
			}

			if cmd.Name() == "init" || cmd.Name() == "add" {
				return nil
			}

			resolved, err := resolveAPIURL(cmd, opts)
			if err != nil {
				return err
			}

			opts.resolvedAPIURL = resolved
			opts.apiToken = ""
			if resolved != "" {
				token, err := loadCredential(resolved)
				if err != nil && !errors.Is(err, errCredentialsNotFound) {
					return err
				}
				opts.apiToken = token
			}

			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&opts.apiURL, "api-url", "", "ENVY API base URL")
	rootCmd.Flags().BoolVar(&opts.showVersion, "version", false, "print version and exit")

	rootCmd.AddCommand(newInitCommand(opts))
	rootCmd.AddCommand(newLoginCommand(opts))
	rootCmd.AddCommand(newAddCommand(opts))
	rootCmd.AddCommand(newDiffCommand(opts))
	rootCmd.AddCommand(newPullCommand(opts))
	rootCmd.AddCommand(newExportCommand(opts))
	rootCmd.AddCommand(newRunCommand(opts))

	return rootCmd
}

func resolveAPIURL(cmd *cobra.Command, opts *rootOptions) (string, error) {
	if flag := cmd.Flag("api-url"); flag != nil && flag.Changed {
		return opts.apiURL, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	projectConfig, err := config.LoadProjectConfig(cwd)
	if err == nil && projectConfig.APIURL != "" {
		return projectConfig.APIURL, nil
	}
	if err != nil && !errors.Is(err, config.ErrProjectConfigNotFound) {
		return "", err
	}

	return os.Getenv("ENVY_API_URL"), nil
}

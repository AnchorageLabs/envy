package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd is the base command for the ENVY CLI.
var rootCmd = &cobra.Command{
	Use:   "envy",
	Short: "ENVY CLI for managing environment variables",
}

// Execute runs the root command. It returns the process exit code.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		// Commands set their own exit codes via SilenceErrors + RunE returning
		// an exitError. Fall back to 2 for unexpected failures.
		if ee, ok := err.(*exitError); ok {
			return ee.code
		}
		return 2
	}
	return 0
}

// Main is a convenience entrypoint usable from package main.
func Main() {
	os.Exit(Execute())
}

func init() {
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(doctorCmd)
}

// exitError carries a specific process exit code up to Execute.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return "command failed"
}

func newExitError(code int, err error) *exitError {
	return &exitError{code: code, err: err}
}

package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	envfile "envy/cli/internal/env"
	"envy/cli/internal/schema"
)

var doctorCmd = &cobra.Command{
	Use:           "doctor",
	Short:         "Validate .env.local against the local schema",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoctor(cmd, args)
	},
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	sch, err := schema.Load("")
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return newExitError(2, err)
	}

	values, err := envfile.Load("")
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return newExitError(2, err)
	}

	var errorsList []string
	var warnings []string

	// Required missing + type validation for present values.
	for _, field := range sch.Fields {
		val, present := values[field.Name]
		if !present {
			if field.Required {
				errorsList = append(errorsList, fmt.Sprintf("%s: required variable is missing", field.Name))
			}
			continue
		}
		if field.Deprecated {
			warnings = append(warnings, fmt.Sprintf("%s: variable is deprecated", field.Name))
		}
		if verr := field.ValidateValue(val); verr != nil {
			errorsList = append(errorsList, fmt.Sprintf("%s: %s", field.Name, verr.Error()))
		}
	}

	// Unknown variables (not in schema).
	for name := range values {
		if _, ok := sch.FieldByName(name); !ok {
			warnings = append(warnings, fmt.Sprintf("%s: variable is not defined in the schema", name))
		}
	}

	sort.Strings(errorsList)
	sort.Strings(warnings)

	if len(errorsList) > 0 {
		fmt.Fprintf(out, "Errors (%d):\n", len(errorsList))
		for _, e := range errorsList {
			fmt.Fprintf(out, "  - %s\n", e)
		}
	}

	if len(warnings) > 0 {
		fmt.Fprintf(out, "Warnings (%d):\n", len(warnings))
		for _, w := range warnings {
			fmt.Fprintf(out, "  - %s\n", w)
		}
	}

	if len(errorsList) == 0 && len(warnings) == 0 {
		fmt.Fprintln(out, "All good. No issues found.")
	}

	if len(errorsList) > 0 {
		return newExitError(1, fmt.Errorf("%d validation error(s)", len(errorsList)))
	}

	return nil
}

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AnchorageLabs/envy/cli/internal/api"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newProposeCommand(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "propose <message>",
		Short: "Propose schema and value changes to the bound environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			message := strings.TrimSpace(args[0])
			if message == "" {
				return diffOperationalError(fmt.Errorf("proposal message is required"))
			}

			cwd, err := os.Getwd()
			if err != nil {
				return diffOperationalError(err)
			}

			envyDir, err := findEnvyDir(cwd)
			if err != nil {
				return diffOperationalError(err)
			}

			localRaw, err := os.ReadFile(filepath.Join(envyDir, "schema.json"))
			if err != nil {
				return diffOperationalError(fmt.Errorf("load local schema: %w", err))
			}

			projectSlug, environmentName, err := loadEnvironmentBinding(envyDir)
			if err != nil {
				return diffOperationalError(err)
			}

			client := api.NewClient(opts.resolvedAPIURL, opts.apiToken, nil)
			remoteRaw, err := client.GetEnvironmentSchema(projectSlug, environmentName)
			if err != nil {
				return diffOperationalError(err)
			}

			localSchema, err := normalizeSchema(localRaw)
			if err != nil {
				return diffOperationalError(fmt.Errorf("parse local schema: %w", err))
			}
			remoteSchema, err := normalizeSchema(remoteRaw)
			if err != nil {
				return diffOperationalError(fmt.Errorf("parse remote schema: %w", err))
			}

			changes := buildProposalChanges(remoteSchema, localSchema)
			if len(changes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "nothing to propose")
				return nil
			}

			// Load non-secret values from .env.local (never read secret values from disk).
			localValues := loadEnvLocalValues(filepath.Join(filepath.Dir(envyDir), ".env.local"))

			reader := bufio.NewReader(cmd.InOrStdin())
			for i := range changes {
				change := &changes[i]
				if change.Op == "remove" {
					continue
				}

				localEntry := localSchema[change.Key]
				if localEntry.Secret {
					// Secret value updates must come from interactive prompt only.
					if !isInteractive(cmd) {
						// Skip value prompt in non-TTY environments to avoid hangs.
						continue
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "Enter value for secret %q (leave blank to skip): ", change.Key)
					line, _ := reader.ReadString('\n')
					value := strings.TrimRight(line, "\r\n")
					if value != "" {
						change.Value = value
						change.HasValue = true
					}
					continue
				}

				// Non-secret value updates: include if .env.local differs from draft.
				if value, ok := localValues[change.Key]; ok {
					change.Value = value
					change.HasValue = true
				}
			}

			payload := make([]api.ProposalChange, 0, len(changes))
			for _, change := range changes {
				apiChange := api.ProposalChange{
					Op:  change.Op,
					Key: change.Key,
				}
				if change.HasType {
					apiChange.Type = change.Type
				}
				if change.HasRequired {
					required := change.Required
					apiChange.Required = &required
				}
				if change.HasSecret {
					secret := change.Secret
					apiChange.Secret = &secret
				}
				if change.HasValue {
					value := change.Value
					apiChange.Value = &value
				}
				payload = append(payload, apiChange)
			}

			proposal, err := client.CreateProposal(projectSlug, environmentName, message, payload)
			if err != nil {
				return diffOperationalError(err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Proposal #%d created\n", proposal.ID)
			return nil
		},
	}

	return cmd
}

type proposalChange struct {
	Op  string
	Key string

	Type        string
	HasType     bool
	Required    bool
	HasRequired bool
	Secret      bool
	HasSecret   bool

	Value    string
	HasValue bool
}

// buildProposalChanges translates the schema diff into add/update/remove ops.
func buildProposalChanges(remote map[string]schemaEntry, local map[string]schemaEntry) []proposalChange {
	keySet := map[string]bool{}
	for key := range remote {
		keySet[key] = true
	}
	for key := range local {
		keySet[key] = true
	}

	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	changes := []proposalChange{}
	for _, key := range keys {
		remoteEntry, hasRemote := remote[key]
		localEntry, hasLocal := local[key]

		switch {
		case hasLocal && !hasRemote:
			changes = append(changes, proposalChange{
				Op:          "add",
				Key:         key,
				Type:        typeLabel(localEntry.Type),
				HasType:     true,
				Required:    localEntry.Required,
				HasRequired: true,
				Secret:      localEntry.Secret,
				HasSecret:   true,
			})
		case hasRemote && !hasLocal:
			changes = append(changes, proposalChange{
				Op:  "remove",
				Key: key,
			})
		case hasRemote && hasLocal:
			if schemaEntriesEqual(remoteEntry, localEntry) {
				continue
			}
			change := proposalChange{Op: "update", Key: key}
			if remoteEntry.Type != localEntry.Type {
				change.Type = typeLabel(localEntry.Type)
				change.HasType = true
			}
			if remoteEntry.Required != localEntry.Required {
				change.Required = localEntry.Required
				change.HasRequired = true
			}
			if remoteEntry.Secret != localEntry.Secret {
				change.Secret = localEntry.Secret
				change.HasSecret = true
			}
			changes = append(changes, change)
		}
	}

	return changes
}

func schemaEntriesEqual(a schemaEntry, b schemaEntry) bool {
	return a.Type == b.Type && a.Required == b.Required && a.Secret == b.Secret
}

// loadEnvLocalValues parses a .env.local file into a key->value map.
// Missing files yield an empty map rather than an error.
func loadEnvLocalValues(path string) map[string]string {
	values := map[string]string{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return values
	}

	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		value := strings.TrimSpace(line[eq+1:])
		value = unquoteEnvValue(value)
		if key != "" {
			values[key] = value
		}
	}

	return values
}

func unquoteEnvValue(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func isInteractive(cmd *cobra.Command) bool {
	file, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

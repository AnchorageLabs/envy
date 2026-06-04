package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AnchorageLabs/envy/cli/internal/api"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type diffExitError struct {
	code int
	err  error
}

func (e *diffExitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *diffExitError) Unwrap() error {
	return e.err
}

func (e *diffExitError) ExitCode() int {
	return e.code
}

type schemaEntry struct {
	Type     string
	Required bool
	Secret   bool
}

func newDiffCommand(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare local schema with the bound environment draft schema",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

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

			lines := buildSchemaDiff(remoteSchema, localSchema, colorEnabled(cmd.OutOrStdout()))
			if len(lines) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "up to date")
				return nil
			}

			for _, line := range lines {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}

			cmd.SilenceErrors = true
			return &diffExitError{code: 1}
		},
	}

	return cmd
}

func diffOperationalError(err error) error {
	return &diffExitError{code: 2, err: err}
}

func findEnvyDir(start string) (string, error) {
	dir := start
	for {
		candidate := filepath.Join(dir, ".envy")
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf(".envy directory not found")
		}
		dir = parent
	}
}

func loadEnvironmentBinding(envyDir string) (string, string, error) {
	candidates := []string{
		"config.json",
		"project.json",
		"environment.json",
		"binding.json",
		"envy.json",
	}

	var projectSlug string
	var environmentName string
	for _, name := range candidates {
		path := filepath.Join(envyDir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", "", err
		}

		var doc any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&doc); err != nil {
			return "", "", fmt.Errorf("parse %s: %w", path, err)
		}

		if projectSlug == "" {
			projectSlug = firstStringAtAnyPath(doc,
				[]string{"projectSlug"},
				[]string{"project_slug"},
				[]string{"project", "slug"},
				[]string{"project", "name"},
				[]string{"slug"},
			)
		}
		if environmentName == "" {
			environmentName = firstStringAtAnyPath(doc,
				[]string{"environmentName"},
				[]string{"environment_name"},
				[]string{"environment", "name"},
				[]string{"environment", "slug"},
				[]string{"environment"},
				[]string{"env"},
			)
		}
	}

	if projectSlug == "" || environmentName == "" {
		return "", "", fmt.Errorf("bound project/environment not found in .envy config")
	}

	return projectSlug, environmentName, nil
}

func firstStringAtAnyPath(doc any, paths ...[]string) string {
	for _, path := range paths {
		if value := stringAtPath(doc, path); value != "" {
			return value
		}
	}
	return ""
}

func stringAtPath(doc any, path []string) string {
	current := doc
	for _, segment := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = m[segment]
		if !ok {
			return ""
		}
	}

	value, ok := current.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func normalizeSchema(raw []byte) (map[string]schemaEntry, error) {
	var doc any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}

	doc = unwrapSchemaDocument(doc)
	entries := map[string]schemaEntry{}
	collectSchemaEntries(doc, entries)
	return entries, nil
}

func unwrapSchemaDocument(doc any) any {
	for {
		if text, ok := doc.(string); ok {
			trimmed := strings.TrimSpace(text)
			if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
				var nested any
				decoder := json.NewDecoder(strings.NewReader(trimmed))
				decoder.UseNumber()
				if decoder.Decode(&nested) == nil {
					doc = nested
					continue
				}
			}
			return doc
		}

		m, ok := doc.(map[string]any)
		if !ok {
			return doc
		}

		unwrapped := false
		for _, key := range []string{"schema", "draftSchema", "draft_schema", "draft", "data", "result"} {
			if nested, ok := m[key]; ok {
				doc = nested
				unwrapped = true
				break
			}
		}
		if !unwrapped {
			return doc
		}
	}
}

func collectSchemaEntries(doc any, entries map[string]schemaEntry) {
	switch value := doc.(type) {
	case []any:
		for _, item := range value {
			if name, entry, ok := schemaEntryFromValue("", item); ok {
				entries[name] = entry
			}
		}
	case map[string]any:
		if properties, ok := value["properties"]; ok {
			collectNamedSchemaEntries(properties, entries, stringSetFromJSON(value["required"]))
			return
		}

		for _, key := range []string{"variables", "fields", "entries", "keys"} {
			if nested, ok := value[key]; ok {
				collectNamedSchemaEntries(nested, entries, nil)
				return
			}
		}

		if name, entry, ok := schemaEntryFromValue("", value); ok {
			entries[name] = entry
			return
		}

		collectNamedSchemaEntries(value, entries, nil)
	}
}

func collectNamedSchemaEntries(doc any, entries map[string]schemaEntry, requiredNames map[string]bool) {
	switch value := doc.(type) {
	case []any:
		for _, item := range value {
			if name, entry, ok := schemaEntryFromValue("", item); ok {
				if requiredNames != nil && requiredNames[name] {
					entry.Required = true
				}
				entries[name] = entry
			}
		}
	case map[string]any:
		for name, item := range value {
			if shouldSkipSchemaMapKey(name) {
				continue
			}
			if entryName, entry, ok := schemaEntryFromValue(name, item); ok {
				if requiredNames != nil && requiredNames[entryName] {
					entry.Required = true
				}
				entries[entryName] = entry
			}
		}
	}
}

func schemaEntryFromValue(name string, value any) (string, schemaEntry, bool) {
	entry := schemaEntry{}
	name = strings.TrimSpace(name)

	switch typed := value.(type) {
	case string:
		entry.Type = strings.TrimSpace(typed)
	case map[string]any:
		if name == "" {
			name = firstStringInMap(typed, "name", "key", "variable")
		}
		entry.Type = firstStringInMap(typed, "type", "kind", "valueType", "value_type")
		entry.Required = firstBoolInMap(typed, false, "required", "isRequired", "is_required")
		entry.Secret = firstBoolInMap(typed, false, "secret", "isSecret", "is_secret")
	default:
		return "", schemaEntry{}, false
	}

	if name == "" {
		return "", schemaEntry{}, false
	}
	return name, entry, true
}

func firstStringInMap(m map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		s, ok := value.(string)
		if !ok {
			continue
		}
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstBoolInMap(m map[string]any, defaultValue bool, keys ...string) bool {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "true", "yes", "1":
				return true
			case "false", "no", "0":
				return false
			}
		}
	}
	return defaultValue
}

func stringSetFromJSON(value any) map[string]bool {
	items, ok := value.([]any)
	if !ok {
		return nil
	}

	set := map[string]bool{}
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			set[text] = true
		}
	}
	return set
}

func shouldSkipSchemaMapKey(key string) bool {
	switch key {
	case "schema", "draftSchema", "draft_schema", "draft", "data", "result", "variables", "fields", "entries", "keys", "properties", "required", "type", "version", "createdAt", "created_at", "updatedAt", "updated_at", "project", "environment":
		return true
	default:
		return false
	}
}

func buildSchemaDiff(remote map[string]schemaEntry, local map[string]schemaEntry, color bool) []string {
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

	lines := []string{}
	for _, key := range keys {
		remoteEntry, hasRemote := remote[key]
		localEntry, hasLocal := local[key]

		switch {
		case hasLocal && !hasRemote:
			line := fmt.Sprintf("+ %s (%s, %s, %s)", key, typeLabel(localEntry.Type), requiredLabel(localEntry.Required), secretLabel(localEntry.Secret))
			lines = append(lines, colorize(line, "32", color))
		case hasRemote && !hasLocal:
			lines = append(lines, colorize("- "+key, "31", color))
		case hasRemote && hasLocal:
			lines = append(lines, changedFieldLines(key, remoteEntry, localEntry, color)...)
		}
	}

	return lines
}

func changedFieldLines(key string, remote schemaEntry, local schemaEntry, color bool) []string {
	lines := []string{}
	if remote.Type != local.Type {
		lines = append(lines, colorize(fmt.Sprintf("~ %s: type %s → %s", key, typeLabel(remote.Type), typeLabel(local.Type)), "33", color))
	}
	if remote.Required != local.Required {
		lines = append(lines, colorize(fmt.Sprintf("~ %s: required %s → %s", key, requiredLabel(remote.Required), requiredLabel(local.Required)), "33", color))
	}
	if remote.Secret != local.Secret {
		lines = append(lines, colorize(fmt.Sprintf("~ %s: secret %s → %s", key, secretLabel(remote.Secret), secretLabel(local.Secret)), "33", color))
	}
	return lines
}

func typeLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return strings.TrimSpace(value)
}

func requiredLabel(value bool) string {
	if value {
		return "required"
	}
	return "optional"
}

func secretLabel(value bool) string {
	if value {
		return "secret"
	}
	return "plain"
}

func colorize(text string, code string, enabled bool) string {
	if !enabled {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func colorEnabled(out io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}

	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

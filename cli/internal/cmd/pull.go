package cmd

import (
	"crypto/sha256"
	"encoding/hex"
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

type pullOptions struct {
	root  *rootOptions
	force bool
}

// lockFile mirrors the committable shape of .envy/lock.json per docs/FILES.md.
type lockFile struct {
	Project      string                  `json:"project"`
	Environments map[string]lockEnvBlock `json:"environments"`
}

type lockEnvBlock struct {
	Version  int                       `json:"version"`
	Checksum string                    `json:"checksum"`
	Keys     map[string]lockKeyMeta    `json:"keys"`
}

type lockKeyMeta struct {
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Secret   bool   `json:"secret"`
}

// schemaEntry mirrors the per-variable schema metadata returned by the API and
// described in docs/FILES.md (.envy/schema.json shape).
type schemaEntry struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Secret   bool   `json:"secret"`
}

func newPullCommand(root *rootOptions) *cobra.Command {
	opts := &pullOptions{root: root}

	cmd := &cobra.Command{
		Use:   "pull [env]",
		Short: "Fetch the stable version's values into .env.local and update the lockfile",
		Long:  "Fetch the stable published version for an environment, write a sorted .env.local, and update .envy/lock.json with the version, checksum, and per-key metadata.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPull(cmd, opts, args)
		},
	}

	cmd.Flags().BoolVar(&opts.force, "force", false, "overwrite .env.local even if it has diverged from the lockfile")

	return cmd
}

func runPull(cmd *cobra.Command, opts *pullOptions, args []string) error {
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

	envName := ""
	if len(args) == 1 {
		envName = strings.TrimSpace(args[0])
	}
	if envName == "" {
		envName = strings.TrimSpace(projectConfig.Environment)
	}
	if envName == "" {
		return fmt.Errorf("no environment specified: pass one as an argument or set a default environment with `envy init`")
	}

	if strings.TrimSpace(opts.root.resolvedAPIURL) == "" {
		return fmt.Errorf("api url is required: configure it in .envy/config.json, set ENVY_API_URL, or pass --api-url")
	}

	client := api.NewClient(opts.root.resolvedAPIURL, opts.root.apiToken, nil)

	env, err := client.GetEnvironment(projectConfig.Project, envName)
	if err != nil {
		return err
	}

	schemaRaw, err := client.GetEnvironmentSchema(projectConfig.Project, envName)
	if err != nil {
		return err
	}

	values, err := client.GetEnvironmentVersionValues(projectConfig.Project, envName, env.StableVersion)
	if err != nil {
		return err
	}

	schema, err := parseSchema(schemaRaw)
	if err != nil {
		return err
	}

	schemaByKey := make(map[string]schemaEntry, len(schema))
	for _, s := range schema {
		schemaByKey[s.Key] = s
	}

	// Build the sorted set of keys from the fetched version's values.
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	envLocalPath := filepath.Join(cwd, ".env.local")
	lockPath := filepath.Join(cwd, ".envy", "lock.json")

	checksum := computeChecksum(keys, values, schemaByKey)

	// Drift detection: refuse if the existing .env.local diverges from the
	// recorded lockfile state, unless --force is set.
	if !opts.force {
		drifted, err := detectDrift(envLocalPath, lockPath, envName)
		if err != nil {
			return err
		}
		if drifted {
			return fmt.Errorf(".env.local has diverged from the lockfile: re-run with --force to overwrite local changes")
		}
	}

	if err := writeEnvLocal(envLocalPath, keys, values); err != nil {
		return err
	}

	if err := writeLockFile(lockPath, projectConfig.Project, envName, env.StableVersion, checksum, keys, schemaByKey, values); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Synced %d keys from %s@%d\n", len(keys), envName, env.StableVersion)
	return nil
}

func parseSchema(raw json.RawMessage) ([]schemaEntry, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var schema []schemaEntry
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("failed to parse schema response: %w", err)
	}
	return schema, nil
}

// computeChecksum implements the algorithm defined in docs/FILES.md.
func computeChecksum(sortedKeys []string, values map[string]string, schemaByKey map[string]schemaEntry) string {
	entries := make([]string, 0, len(sortedKeys))
	for _, key := range sortedKeys {
		value := values[key]
		if schemaByKey[key].Secret {
			sum := sha256.Sum256([]byte(value))
			entries = append(entries, key+"="+hex.EncodeToString(sum[:]))
		} else {
			entries = append(entries, key+"="+value)
		}
	}
	joined := strings.Join(entries, "\n")
	digest := sha256.Sum256([]byte(joined))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func writeEnvLocal(path string, sortedKeys []string, values map[string]string) error {
	var b strings.Builder
	for _, key := range sortedKeys {
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(values[key])
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func writeLockFile(path string, project string, envName string, version int, checksum string, sortedKeys []string, schemaByKey map[string]schemaEntry, values map[string]string) error {
	lock := lockFile{Environments: map[string]lockEnvBlock{}}

	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &lock); err != nil {
			return fmt.Errorf("failed to parse existing lock.json: %w", err)
		}
		if lock.Environments == nil {
			lock.Environments = map[string]lockEnvBlock{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	lock.Project = project

	keyMeta := make(map[string]lockKeyMeta, len(sortedKeys))
	for _, key := range sortedKeys {
		s := schemaByKey[key]
		keyMeta[key] = lockKeyMeta{
			Type:     s.Type,
			Required: s.Required,
			Secret:   s.Secret,
		}
	}

	lock.Environments[envName] = lockEnvBlock{
		Version:  version,
		Checksum: checksum,
		Keys:     keyMeta,
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	return os.WriteFile(path, encoded, 0o644)
}

// detectDrift returns true if .env.local exists and its content no longer
// matches the checksum recorded in the lockfile for the target environment.
// If there is no lockfile entry or no .env.local, there is no drift to guard
// against (a fresh pull).
func detectDrift(envLocalPath string, lockPath string, envName string) (bool, error) {
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	var lock lockFile
	if err := json.Unmarshal(lockData, &lock); err != nil {
		return false, fmt.Errorf("failed to parse existing lock.json: %w", err)
	}

	block, ok := lock.Environments[envName]
	if !ok {
		return false, nil
	}

	localData, err := os.ReadFile(envLocalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	localKeys, localValues := parseEnvLocal(localData)
	schemaByKey := make(map[string]schemaEntry, len(block.Keys))
	for key, meta := range block.Keys {
		schemaByKey[key] = schemaEntry{Key: key, Type: meta.Type, Required: meta.Required, Secret: meta.Secret}
	}

	localChecksum := computeChecksum(localKeys, localValues, schemaByKey)
	return localChecksum != block.Checksum, nil
}

// parseEnvLocal parses KEY=VALUE lines, ignoring comments and blank lines, and
// returns a sorted key slice and a value map.
func parseEnvLocal(data []byte) ([]string, map[string]string) {
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		if key == "" {
			continue
		}
		values[key] = line[idx+1:]
	}

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, values
}

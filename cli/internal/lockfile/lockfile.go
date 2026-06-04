package lockfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultPath is the conventional location of the ENVY lockfile.
const DefaultPath = ".envy/lock.json"

// ErrNotFound indicates the lockfile does not exist (not yet synced).
var ErrNotFound = errors.New("lockfile not found")

// Lockfile represents the contents of .envy/lock.json.
//
// NOTE: Field names are based on the plan's stated assumptions. Confirm
// against docs/MODEL.md and the envy pull/diff implementation.
type Lockfile struct {
	Environment string `json:"environment"`
	Project     string `json:"project"`
	VersionID   string `json:"version_id"`
	Version     int    `json:"version"`
}

// Load reads and parses the lockfile at path. If path is empty, DefaultPath is used.
// Returns ErrNotFound if the file does not exist.
func Load(path string) (*Lockfile, error) {
	if path == "" {
		path = DefaultPath
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to read lockfile: %w", err)
	}

	var lf Lockfile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("failed to parse lockfile: %w", err)
	}

	return &lf, nil
}

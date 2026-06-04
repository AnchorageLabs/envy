package env

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPath is the conventional location of the local env file.
const DefaultPath = ".env.local"

// ErrNotFound indicates the env file does not exist.
var ErrNotFound = errors.New(".env.local not found")

// Load reads and parses a dotenv-style file into an ordered key/value map.
// If path is empty, DefaultPath is used. Returns ErrNotFound if missing.
func Load(path string) (map[string]string, error) {
	if path == "" {
		path = DefaultPath
	}

	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to read env file: %w", err)
	}
	defer f.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.Index(line, "=")
		if eq < 0 {
			return nil, fmt.Errorf("invalid line %d in env file: missing '='", lineNum)
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = unquote(val)
		if key == "" {
			return nil, fmt.Errorf("invalid line %d in env file: empty key", lineNum)
		}
		values[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan env file: %w", err)
	}

	return values, nil
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

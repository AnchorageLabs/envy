package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// DefaultPath is the conventional location of the local ENVY schema.
const DefaultPath = ".envy/schema.json"

// ErrNotFound indicates the schema file does not exist.
var ErrNotFound = errors.New("schema not found")

// Field describes a single variable in the schema.
//
// The canonical on-disk shape (see docs/FILES.md) uses `key` for the variable
// name. We unmarshal both `key` and `name` for resilience and normalize to the
// Name field after parsing.
type Field struct {
	Name       string   `json:"-"`
	Key        string   `json:"key,omitempty"`
	NameAlias  string   `json:"name,omitempty"`
	Type       string   `json:"type"`
	Required   bool     `json:"required"`
	Secret     bool     `json:"secret"`
	Deprecated bool     `json:"deprecated"`
	Enum       []string `json:"enum,omitempty"`
}

// Schema is the parsed local schema file.
type Schema struct {
	Fields []Field `json:"fields"`
}

// Load reads and parses the schema at path. If path is empty, DefaultPath is used.
// Returns ErrNotFound if the file does not exist.
func Load(path string) (*Schema, error) {
	if path == "" {
		path = DefaultPath
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to read schema: %w", err)
	}

	return Parse(data)
}

// Parse parses raw schema JSON. It supports two shapes:
//
//   1. The canonical top-level array of fields (docs/FILES.md).
//   2. An object with a `fields` array (legacy/internal shape).
func Parse(data []byte) (*Schema, error) {
	trimmed := bytes.TrimSpace(data)

	var s Schema

	if len(trimmed) > 0 && trimmed[0] == '[' {
		var fields []Field
		if err := json.Unmarshal(trimmed, &fields); err != nil {
			return nil, fmt.Errorf("failed to parse schema: %w", err)
		}
		s.Fields = fields
	} else {
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, fmt.Errorf("failed to parse schema: %w", err)
		}
	}

	for i := range s.Fields {
		s.Fields[i].normalize()
	}

	return &s, nil
}

// normalize resolves the variable name from `key` or `name`.
func (f *Field) normalize() {
	if f.Key != "" {
		f.Name = f.Key
		return
	}
	if f.NameAlias != "" {
		f.Name = f.NameAlias
	}
}

// FieldByName returns the field with the given name, if present.
func (s *Schema) FieldByName(name string) (Field, bool) {
	for _, f := range s.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// ValidateValue validates a single value against a field's type constraints.
// Returns an error describing the mismatch, or nil if valid.
func (f Field) ValidateValue(value string) error {
	switch f.Type {
	case "enum":
		for _, allowed := range f.Enum {
			if value == allowed {
				return nil
			}
		}
		return fmt.Errorf("value %q is not a valid enum member (allowed: %v)", value, f.Enum)
	case "number":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("value %q is not a valid number", value)
		}
		return nil
	case "boolean":
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("value %q is not a valid boolean", value)
		}
		return nil
	case "", "string":
		return nil
	default:
		// Unknown type defined in schema: accept rather than block.
		return nil
	}
}

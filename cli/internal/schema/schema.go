package schema

import (
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
// NOTE: Field shape is based on the plan's stated assumptions. Confirm
// against docs/MODEL.md. Type is one of: string, number, enum (and any
// other types defined in schema.json).
type Field struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Required   bool     `json:"required"`
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

// Parse parses raw schema JSON.
func Parse(data []byte) (*Schema, error) {
	var s Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to parse schema: %w", err)
	}
	return &s, nil
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
	case "", "string":
		return nil
	default:
		// Unknown type defined in schema: accept rather than block.
		return nil
	}
}

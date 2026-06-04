package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var schemaKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

type addOptions struct {
	valueType   string
	secret      bool
	required    bool
	defaultRaw  string
	description string
	owner       string
	force       bool
}

type schemaVariable struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"`
	Default     any    `json:"default"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
}

type schemaDocument struct {
	format    string
	variables []schemaVariable
	object    map[string]json.RawMessage
}

func newAddCommand(opts *rootOptions) *cobra.Command {
	addOpts := &addOptions{}

	cmd := &cobra.Command{
		Use:   "add KEY",
		Short: "Add a variable to the local schema",
		Long:  "Add a variable entry to .envy/schema.json, or update an existing entry with --force.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defaultChanged := false
			if flag := cmd.Flags().Lookup("default"); flag != nil {
				defaultChanged = flag.Changed
			}

			return runAdd(cmd, args[0], addOpts, defaultChanged)
		},
	}

	cmd.Flags().StringVar(&addOpts.valueType, "type", "string", "variable type: string, boolean, number, or enum:foo|bar")
	cmd.Flags().BoolVar(&addOpts.secret, "secret", false, "mark the variable as secret")
	cmd.Flags().BoolVar(&addOpts.required, "required", false, "mark the variable as required")
	cmd.Flags().StringVar(&addOpts.defaultRaw, "default", "", "default metadata value")
	cmd.Flags().StringVar(&addOpts.description, "description", "", "variable description")
	cmd.Flags().StringVar(&addOpts.owner, "owner", "", "variable owner")
	cmd.Flags().BoolVar(&addOpts.force, "force", false, "update the existing variable if it already exists")

	return cmd
}

func runAdd(cmd *cobra.Command, key string, opts *addOptions, defaultChanged bool) error {
	if err := validateSchemaKey(key); err != nil {
		return err
	}

	storedType, enumChoices, err := parseSchemaType(opts.valueType)
	if err != nil {
		return err
	}

	defaultValue, err := parseDefaultValue(storedType, enumChoices, opts.defaultRaw, defaultChanged)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	schemaPath := filepath.Join(cwd, ".envy", "schema.json")
	doc, err := loadSchemaDocument(schemaPath)
	if err != nil {
		return err
	}

	entry := schemaVariable{
		Key:         key,
		Type:        storedType,
		Required:    opts.required,
		Secret:      opts.secret,
		Default:     defaultValue,
		Description: opts.description,
		Owner:       opts.owner,
	}

	index := -1
	for i, variable := range doc.variables {
		if variable.Key == key {
			index = i
			break
		}
	}

	action := "Added"
	if index >= 0 {
		if !opts.force {
			return fmt.Errorf("variable %q already exists in .envy/schema.json; use --force to update it", key)
		}
		doc.variables[index] = entry
		action = "Updated"
	} else {
		doc.variables = append(doc.variables, entry)
	}

	if err := writeSchemaDocument(schemaPath, doc); err != nil {
		return err
	}

	printAddSummary(cmd, action, entry)
	return nil
}

func validateSchemaKey(key string) error {
	if len(key) > 64 {
		return fmt.Errorf("invalid key %q: key length must be <= 64 characters", key)
	}
	if !schemaKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid key %q: key must match ^[A-Z][A-Z0-9_]*$", key)
	}
	return nil
}

func parseSchemaType(valueType string) (string, []string, error) {
	switch valueType {
	case "string", "boolean", "number":
		return valueType, nil, nil
	}

	if !strings.HasPrefix(valueType, "enum:") {
		return "", nil, fmt.Errorf("unsupported type %q: use string, boolean, number, or enum:foo|bar", valueType)
	}

	rawChoices := strings.TrimPrefix(valueType, "enum:")
	if rawChoices == "" {
		return "", nil, errors.New("invalid enum type: at least one enum choice is required")
	}

	parts := strings.Split(rawChoices, "|")
	choices := make([]string, 0, len(parts))
	for _, part := range parts {
		choice := strings.TrimSpace(part)
		if choice == "" {
			return "", nil, fmt.Errorf("invalid enum type %q: enum choices must not be empty", valueType)
		}
		choices = append(choices, choice)
	}

	return "enum:" + strings.Join(choices, "|"), choices, nil
}

func parseDefaultValue(storedType string, enumChoices []string, raw string, changed bool) (any, error) {
	if !changed {
		return nil, nil
	}

	switch {
	case storedType == "string":
		return raw, nil
	case storedType == "boolean":
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid default %q for boolean type: use true or false", raw)
		}
		return parsed, nil
	case storedType == "number":
		if strings.TrimSpace(raw) != raw || raw == "" {
			return nil, fmt.Errorf("invalid default %q for number type", raw)
		}
		if _, err := strconv.ParseFloat(raw, 64); err != nil {
			return nil, fmt.Errorf("invalid default %q for number type", raw)
		}
		return json.Number(raw), nil
	case strings.HasPrefix(storedType, "enum:"):
		for _, choice := range enumChoices {
			if raw == choice {
				return raw, nil
			}
		}
		return nil, fmt.Errorf("invalid default %q for %s: default must be one of the enum choices", raw, storedType)
	default:
		return nil, fmt.Errorf("unsupported type %q", storedType)
	}
}

func loadSchemaDocument(path string) (*schemaDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf(".envy/schema.json not found; run envy init or create the schema file first")
		}
		return nil, fmt.Errorf("read .envy/schema.json: %w", err)
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errors.New(".envy/schema.json is empty")
	}

	switch trimmed[0] {
	case '[':
		var variables []schemaVariable
		if err := decodeJSON(trimmed, &variables); err != nil {
			return nil, fmt.Errorf("parse .envy/schema.json: %w", err)
		}
		return &schemaDocument{format: "array", variables: variables}, nil
	case '{':
		var object map[string]json.RawMessage
		if err := decodeJSON(trimmed, &object); err != nil {
			return nil, fmt.Errorf("parse .envy/schema.json: %w", err)
		}

		rawVariables, ok := object["variables"]
		if !ok {
			return nil, errors.New(".envy/schema.json object must contain a variables array")
		}

		var variables []schemaVariable
		if err := decodeJSON(rawVariables, &variables); err != nil {
			return nil, fmt.Errorf("parse .envy/schema.json variables: %w", err)
		}

		return &schemaDocument{format: "object", variables: variables, object: object}, nil
	default:
		return nil, errors.New(".envy/schema.json must be a JSON array of variables")
	}
}

func writeSchemaDocument(path string, doc *schemaDocument) error {
	var (
		data []byte
		err  error
	)

	if doc.format == "object" {
		variablesData, err := json.Marshal(doc.variables)
		if err != nil {
			return fmt.Errorf("encode schema variables: %w", err)
		}
		doc.object["variables"] = variablesData
		data, err = json.MarshalIndent(doc.object, "", "  ")
	} else {
		data, err = json.MarshalIndent(doc.variables, "", "  ")
	}
	if err != nil {
		return fmt.Errorf("encode .envy/schema.json: %w", err)
	}

	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write .envy/schema.json: %w", err)
	}

	return nil
}

func decodeJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func printAddSummary(cmd *cobra.Command, action string, entry schemaVariable) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s %s\n", action, entry.Key)
	fmt.Fprintf(out, "  type: %s\n", entry.Type)
	fmt.Fprintf(out, "  required: %t\n", entry.Required)
	fmt.Fprintf(out, "  secret: %t\n", entry.Secret)
	fmt.Fprintf(out, "  default: %s\n", formatDefaultSummary(entry.Default))
	if entry.Description != "" {
		fmt.Fprintf(out, "  description: %s\n", entry.Description)
	}
	if entry.Owner != "" {
		fmt.Fprintf(out, "  owner: %s\n", entry.Owner)
	}
}

func formatDefaultSummary(value any) string {
	if value == nil {
		return "<none>"
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

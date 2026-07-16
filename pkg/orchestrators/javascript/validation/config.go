package workflowvalidation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func validateConfig(req Request) []Issue {
	var issues []Issue
	issues = append(issues, validateMetadata(req)...)
	issues = append(issues, validateArgsSchema(req)...)
	return issues
}

// ValidateArgs applies an authored JavaScript workflow argument schema before
// execution so invalid invocation input cannot create workflow side effects.
func ValidateArgs(schemaJSON json.RawMessage, args map[string]any) error {
	if len(schemaJSON) == 0 || strings.TrimSpace(string(schemaJSON)) == "null" {
		return nil
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return fmt.Errorf("workflow args schema is invalid: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	const schemaID = "workflow-args-schema.json"
	if err := compiler.AddResource(schemaID, document); err != nil {
		return fmt.Errorf("workflow args schema is invalid: %w", err)
	}
	schema, err := compiler.Compile(schemaID)
	if err != nil {
		return fmt.Errorf("workflow args schema is invalid: %w", err)
	}
	if err := schema.Validate(args); err != nil {
		return fmt.Errorf("workflow args do not satisfy argsSchema: %w", err)
	}
	return nil
}

func validateMetadata(req Request) []Issue {
	if len(req.Metadata) == 0 {
		return nil
	}
	var issues []Issue
	for key, value := range req.Metadata {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			issues = append(issues, Issue{
				Code:    CodeInvalidMetadata,
				Message: "orchestrator.javascript.metadata contains an empty key",
				Path:    configPath(req, "metadata"),
			})
			continue
		}
		if strings.TrimSpace(value) == "" {
			issues = append(issues, Issue{
				Code:    CodeInvalidMetadata,
				Message: "orchestrator.javascript.metadata[" + trimmedKey + "] must be a non-empty string",
				Path:    configPath(req, "metadata."+trimmedKey),
			})
		}
	}
	return issues
}

func validateArgsSchema(req Request) []Issue {
	if len(req.ArgsSchema) == 0 {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(req.ArgsSchema, &decoded); err != nil {
		return []Issue{{
			Code:    CodeInvalidArgsSchema,
			Message: "orchestrator.javascript.argsSchema must be valid JSON: " + err.Error(),
			Path:    configPath(req, "argsSchema"),
		}}
	}
	schemaType, ok := decoded["type"].(string)
	if !ok || strings.TrimSpace(schemaType) == "" {
		return []Issue{{
			Code:    CodeInvalidArgsSchema,
			Message: "orchestrator.javascript.argsSchema must declare a JSON Schema type",
			Path:    configPath(req, "argsSchema.type"),
		}}
	}
	if schemaType != "object" {
		return []Issue{{
			Code:    CodeInvalidArgsSchema,
			Message: "orchestrator.javascript.argsSchema.type must be \"object\" for MVP workflows",
			Path:    configPath(req, "argsSchema.type"),
		}}
	}
	return nil
}

func configPath(req Request, suffix string) string {
	base := strings.TrimSpace(req.ConfigPath)
	if base == "" {
		base = "orchestrator.javascript"
	}
	if suffix == "" {
		return base
	}
	return base + "." + suffix
}

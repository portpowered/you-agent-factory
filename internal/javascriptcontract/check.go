package javascriptcontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const (
	// StagedRuntimeCatalogPath is the package-facing copy of the canonical
	// runtime catalog.
	StagedRuntimeCatalogPath = "packages/api/generated/javascript/runtime-api.json"
	generatedContractCommand = "make contracts-generate"
)

// Diagnostic identifies one field-level difference in a generated JavaScript
// runtime projection. Path is repository-relative and Field is the affected
// agent.run property.
type Diagnostic struct {
	Code    string
	Path    string
	Field   string
	Message string
}

type fieldShape struct {
	JSONType string
	Required bool
	Enum     []string
}

// CheckGeneratedOutputs computes the canonical catalog and documentation
// projections in memory, then compares them with the committed outputs. It is
// deliberately read-only; contracts-generate is the corresponding write path.
func CheckGeneratedOutputs(repositoryRoot string) ([]Diagnostic, error) {
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}

	canonical, err := readGeneratedOutput(root, RuntimeCatalogPath)
	if err != nil {
		return nil, err
	}
	expectedCatalog, err := GenerateRuntimeCatalog(canonical)
	if err != nil {
		return nil, fmt.Errorf("generate expected %s: %w", RuntimeCatalogPath, err)
	}

	diagnostics, err := checkCatalogOutput(RuntimeCatalogPath, canonical, expectedCatalog)
	if err != nil {
		return nil, err
	}
	staged, err := readGeneratedOutput(root, StagedRuntimeCatalogPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	} else {
		stagedDiagnostics, err := checkCatalogOutput(StagedRuntimeCatalogPath, staged, expectedCatalog)
		if err != nil {
			return nil, err
		}
		diagnostics = append(diagnostics, stagedDiagnostics...)
	}

	documentation, err := readGeneratedOutput(root, JavaScriptWorkflowReferencePath)
	if err != nil {
		return nil, err
	}
	expectedDocumentation, err := ProjectJavaScriptWorkflowReference(documentation, factoryruntime.JavaScriptChildFieldDescriptors())
	if err != nil {
		return nil, fmt.Errorf("generate expected %s: %w", JavaScriptWorkflowReferencePath, err)
	}
	documentationDiagnostics, err := checkDocumentationOutput(documentation, expectedDocumentation)
	if err != nil {
		return nil, err
	}
	diagnostics = append(diagnostics, documentationDiagnostics...)

	sortDiagnostics(diagnostics)
	return diagnostics, nil
}

func readGeneratedOutput(repositoryRoot, path string) ([]byte, error) {
	payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(path)))
	if err != nil {
		return nil, fmt.Errorf("read generated output %s: %w; run `%s`", path, err, generatedContractCommand)
	}
	return payload, nil
}

func checkCatalogOutput(path string, actual, expected []byte) ([]Diagnostic, error) {
	if bytes.Equal(actual, expected) {
		return nil, nil
	}
	actualFields, err := parseRuntimeCatalogFields(actual)
	if err != nil {
		return nil, fmt.Errorf("inspect stale generated output %s: %w; run `%s`", path, err, generatedContractCommand)
	}
	expectedFields, err := parseRuntimeCatalogFields(expected)
	if err != nil {
		return nil, fmt.Errorf("inspect expected generated output %s: %w", path, err)
	}
	diagnostics := compareFieldShapes(path, "artifact", expectedFields, actualFields)
	if len(diagnostics) > 0 {
		return diagnostics, nil
	}
	return []Diagnostic{{
		Code:    "javascript.generated.output.stale",
		Path:    path,
		Message: fmt.Sprintf("%s differs from the runtime-generated agent.run projection; run `%s`", path, generatedContractCommand),
	}}, nil
}

func checkDocumentationOutput(actual, expected []byte) ([]Diagnostic, error) {
	if bytes.Equal(actual, expected) {
		return nil, nil
	}
	actualFields, err := parseDocumentationFields(actual)
	if err != nil {
		return nil, fmt.Errorf("inspect stale generated output %s: %w; run `%s`", JavaScriptWorkflowReferencePath, err, generatedContractCommand)
	}
	expectedFields, err := parseDocumentationFields(expected)
	if err != nil {
		return nil, fmt.Errorf("inspect expected generated output %s: %w", JavaScriptWorkflowReferencePath, err)
	}
	diagnostics := compareFieldShapes(JavaScriptWorkflowReferencePath, "documentation", expectedFields, actualFields)
	if len(diagnostics) > 0 {
		return diagnostics, nil
	}
	return []Diagnostic{{
		Code:    "javascript.generated.output.stale",
		Path:    JavaScriptWorkflowReferencePath,
		Message: fmt.Sprintf("%s differs from the runtime-generated agent.run documentation projection; run `%s`", JavaScriptWorkflowReferencePath, generatedContractCommand),
	}}, nil
}

func compareFieldShapes(path, surface string, expected, actual map[string]fieldShape) []Diagnostic {
	names := make(map[string]struct{}, len(expected)+len(actual))
	for name := range expected {
		names[name] = struct{}{}
	}
	for name := range actual {
		names[name] = struct{}{}
	}
	orderedNames := make([]string, 0, len(names))
	for name := range names {
		orderedNames = append(orderedNames, name)
	}
	sort.Strings(orderedNames)

	diagnostics := make([]Diagnostic, 0)
	for _, name := range orderedNames {
		want, expectedPresent := expected[name]
		got, actualPresent := actual[name]
		switch {
		case !expectedPresent:
			diagnostics = append(diagnostics, fieldDiagnostic(path, surface, name, "extra", ""))
		case !actualPresent:
			diagnostics = append(diagnostics, fieldDiagnostic(path, surface, name, "missing", ""))
		default:
			if want.JSONType != got.JSONType {
				detail := fmt.Sprintf("got %q, expected %q", got.JSONType, want.JSONType)
				diagnostics = append(diagnostics, fieldDiagnostic(path, surface, name, "type", detail))
			}
			if want.Required != got.Required {
				gotRequiredness := requiredness(got.Required)
				wantRequiredness := requiredness(want.Required)
				detail := fmt.Sprintf("got %s, expected %s", gotRequiredness, wantRequiredness)
				diagnostics = append(diagnostics, fieldDiagnostic(path, surface, name, "requiredness", detail))
			}
			if !equalStrings(want.Enum, got.Enum) {
				detail := fmt.Sprintf("got %v, expected %v", got.Enum, want.Enum)
				diagnostics = append(diagnostics, fieldDiagnostic(path, surface, name, "enum", detail))
			}
		}
	}
	return diagnostics
}

func fieldDiagnostic(path, surface, field, issue, detail string) Diagnostic {
	code := "javascript.agent_run.field." + issue
	message := fmt.Sprintf("%s has %s generated agent.run field %q", path, surface, field)
	switch issue {
	case "missing":
		message += " missing"
	case "extra":
		message += " that is not in the runtime descriptor"
	case "type":
		message += " with a mismatched JSON type"
	case "requiredness":
		message += " with mismatched requiredness"
	case "enum":
		message += " with mismatched allowed values"
	}
	if detail != "" {
		message += " (" + detail + ")"
	}
	message += fmt.Sprintf("; run `%s`", generatedContractCommand)
	return Diagnostic{Code: code, Path: path, Field: field, Message: message}
}

func requiredness(required bool) string {
	if required {
		return "required"
	}
	return "optional"
}

func parseRuntimeCatalogFields(payload []byte) (map[string]fieldShape, error) {
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		return nil, fmt.Errorf("decode %s: %w", RuntimeCatalogPath, err)
	}
	schema, err := agentRunSchema(document)
	if err != nil {
		return nil, err
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s has no object /sharedSchemas/javascript.schema.agent_run_spec/schema/properties", RuntimeCatalogPath)
	}
	requiredValues, ok := schema["required"].([]any)
	if !ok {
		return nil, fmt.Errorf("%s has no array /sharedSchemas/javascript.schema.agent_run_spec/schema/required", RuntimeCatalogPath)
	}
	required := make(map[string]bool, len(requiredValues))
	for _, value := range requiredValues {
		name, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s has a non-string required agent.run field", RuntimeCatalogPath)
		}
		required[name] = true
	}

	fields := make(map[string]fieldShape, len(properties))
	for name, value := range properties {
		property, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s field %q is not an object", RuntimeCatalogPath, name)
		}
		jsonType, ok := property["type"].(string)
		if !ok || jsonType == "" {
			return nil, fmt.Errorf("%s field %q has no JSON type", RuntimeCatalogPath, name)
		}
		enum, err := parseJSONEnum(property["enum"])
		if err != nil {
			return nil, fmt.Errorf("%s field %q has invalid enum: %w", RuntimeCatalogPath, name, err)
		}
		fields[name] = fieldShape{JSONType: jsonType, Required: required[name], Enum: enum}
	}
	for name := range required {
		if _, ok := fields[name]; !ok {
			return nil, fmt.Errorf("%s requires unknown agent.run field %q", RuntimeCatalogPath, name)
		}
	}
	return fields, nil
}

func parseDocumentationFields(document []byte) (map[string]fieldShape, error) {
	region, err := generatedDocumentationRegion(document)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]fieldShape)
	for _, line := range strings.Split(strings.ReplaceAll(string(region), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "### ") || trimmed == "| Field | JSON type | Requiredness | Allowed values |" || trimmed == "|-------|-----------|--------------|----------------|" {
			continue
		}
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		field, shape, err := parseDocumentationRow(trimmed)
		if err != nil {
			return nil, fmt.Errorf("%s has invalid generated agent.run field row: %w", JavaScriptWorkflowReferencePath, err)
		}
		if _, exists := fields[field]; exists {
			return nil, fmt.Errorf("%s repeats generated agent.run field %q", JavaScriptWorkflowReferencePath, field)
		}
		fields[field] = shape
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("%s has no generated agent.run field rows", JavaScriptWorkflowReferencePath)
	}
	return fields, nil
}

func generatedDocumentationRegion(document []byte) ([]byte, error) {
	if _, err := replaceGeneratedAgentRunRegion(document, nil); err != nil {
		return nil, err
	}
	startMarker := bytes.Index(document, []byte(AgentRunFieldsStartMarker))
	endMarker := bytes.Index(document, []byte(AgentRunFieldsEndMarker))
	_, startLineEnd := markdownLineBounds(document, startMarker)
	endLineStart, _ := markdownLineBounds(document, endMarker)
	regionStart := startLineEnd
	for regionStart < len(document) && (document[regionStart] == '\r' || document[regionStart] == '\n') {
		regionStart++
	}
	return document[regionStart:endLineStart], nil
}

func parseDocumentationRow(line string) (string, fieldShape, error) {
	parts := strings.Split(line, "|")
	if len(parts) != 6 {
		return "", fieldShape{}, fmt.Errorf("expected four cells")
	}
	name, err := parseDocumentationCodeCell(parts[1])
	if err != nil {
		return "", fieldShape{}, fmt.Errorf("field name: %w", err)
	}
	jsonType, err := parseDocumentationCodeCell(parts[2])
	if err != nil {
		return "", fieldShape{}, fmt.Errorf("JSON type: %w", err)
	}
	requiredness := strings.TrimSpace(parts[3])
	if requiredness != "required" && requiredness != "optional" {
		return "", fieldShape{}, fmt.Errorf("requiredness %q is invalid", requiredness)
	}
	allowed := strings.TrimSpace(parts[4])
	var enum []string
	if allowed != "—" {
		for _, value := range strings.Split(allowed, ",") {
			value = strings.TrimSpace(value)
			if value == "" {
				return "", fieldShape{}, fmt.Errorf("allowed values contain an empty value")
			}
			enum = append(enum, value)
		}
	}
	return name, fieldShape{JSONType: jsonType, Required: requiredness == "required", Enum: enum}, nil
}

func parseJSONEnum(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("enum is not an array")
	}
	enum := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok || text == "" {
			return nil, fmt.Errorf("enum contains a non-empty string requirement")
		}
		enum = append(enum, text)
	}
	return enum, nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func parseDocumentationCodeCell(cell string) (string, error) {
	trimmed := strings.TrimSpace(cell)
	if len(trimmed) < 2 || trimmed[0] != '`' || trimmed[len(trimmed)-1] != '`' {
		return "", fmt.Errorf("%q is not a code cell", trimmed)
	}
	value := trimmed[1 : len(trimmed)-1]
	if value == "" {
		return "", fmt.Errorf("code cell is empty")
	}
	return value, nil
}

func sortDiagnostics(diagnostics []Diagnostic) {
	sort.Slice(diagnostics, func(left, right int) bool {
		if diagnostics[left].Path != diagnostics[right].Path {
			return diagnostics[left].Path < diagnostics[right].Path
		}
		if diagnostics[left].Field != diagnostics[right].Field {
			return diagnostics[left].Field < diagnostics[right].Field
		}
		if diagnostics[left].Code != diagnostics[right].Code {
			return diagnostics[left].Code < diagnostics[right].Code
		}
		return diagnostics[left].Message < diagnostics[right].Message
	})
}

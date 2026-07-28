package factorydefinition_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	factorydefinitionmcp "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mcp"
)

func TestDiscoverTools_ExposesExpectedFactoryDefinitionTools(t *testing.T) {
	t.Parallel()

	tools := factorydefinitionmcp.DiscoverTools()
	if len(tools) != 4 {
		t.Fatalf("tool count = %d, want 4", len(tools))
	}

	wantNames := []string{
		factorydefinitionmcp.ToolValidate,
		factorydefinitionmcp.ToolGetCurrent,
		factorydefinitionmcp.ToolSaveCurrent,
		factorydefinitionmcp.ToolInstallPackaged,
	}
	gotNames := factorydefinitionmcp.ToolNames()
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("tool names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestDiscoverTools_EachToolHasSchemasDescriptionsAndStableFields(t *testing.T) {
	t.Parallel()

	for _, tool := range factorydefinitionmcp.DiscoverTools() {
		if strings.TrimSpace(tool.Name) == "" {
			t.Fatal("tool name is required")
		}
		if strings.TrimSpace(tool.Description) == "" {
			t.Fatalf("tool %q description is required", tool.Name)
		}
		if len(tool.InputSchema) == 0 {
			t.Fatalf("tool %q input schema is required", tool.Name)
		}
		if schemaType, _ := tool.InputSchema["type"].(string); schemaType != "object" {
			t.Fatalf("tool %q input schema type = %q, want object", tool.Name, schemaType)
		}
		if additional, ok := tool.InputSchema["additionalProperties"].(bool); !ok || additional {
			t.Fatalf("tool %q input schema additionalProperties = %#v, want false", tool.Name, tool.InputSchema["additionalProperties"])
		}
		if len(tool.OutputSchema) == 0 {
			t.Fatalf("tool %q output schema is required", tool.Name)
		}
		if len(tool.SuccessStableFields) == 0 {
			t.Fatalf("tool %q success stable fields are required", tool.Name)
		}
		if len(tool.ErrorStableFields) == 0 {
			t.Fatalf("tool %q error stable fields are required", tool.Name)
		}
		for _, field := range append(tool.SuccessStableFields, tool.ErrorStableFields...) {
			if strings.TrimSpace(field) == "" {
				t.Fatalf("tool %q has empty stable field entry", tool.Name)
			}
		}
	}
}

func TestDiscoverTools_RepresentativeSchemaFields(t *testing.T) {
	t.Parallel()

	byName := toolDefinitionsByName(t)
	validateProps := byName[factorydefinitionmcp.ToolValidate].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"name", "workTypes", "workers", "workstations"} {
		if _, ok := validateProps[field]; !ok {
			t.Fatalf("validate input missing %q", field)
		}
	}

	getCurrentProps := byName[factorydefinitionmcp.ToolGetCurrent].InputSchema["properties"].(map[string]any)
	if _, ok := getCurrentProps["sessionId"]; !ok {
		t.Fatal("get current input missing sessionId")
	}

	saveCurrentProps := byName[factorydefinitionmcp.ToolSaveCurrent].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"sessionId", "mode", "factory"} {
		if _, ok := saveCurrentProps[field]; !ok {
			t.Fatalf("save current input missing %q", field)
		}
	}

	installProps := byName[factorydefinitionmcp.ToolInstallPackaged].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"package", "dir", "format", "replace"} {
		if _, ok := installProps[field]; !ok {
			t.Fatalf("install packaged input missing %q", field)
		}
	}
}

func TestDiscoverTools_InstallPackagedFormatEnumMatchesRuntimeAcceptedValues(t *testing.T) {
	t.Parallel()

	byName := toolDefinitionsByName(t)
	installProps := byName[factorydefinitionmcp.ToolInstallPackaged].InputSchema["properties"].(map[string]any)
	formatSchema, ok := installProps["format"].(map[string]any)
	if !ok {
		t.Fatal("install packaged format schema missing")
	}
	enumValues, ok := formatSchema["enum"].([]string)
	if !ok {
		t.Fatalf("install packaged format enum = %#v, want []string", formatSchema["enum"])
	}

	want := []string{"json", "yaml", "yml"}
	if !slices.Equal(enumValues, want) {
		t.Fatalf("install packaged format enum = %#v, want %#v", enumValues, want)
	}
}

func TestDiscoverTools_OutputSchemasDocumentSharedErrorEnvelope(t *testing.T) {
	t.Parallel()

	for _, tool := range factorydefinitionmcp.DiscoverTools() {
		properties, ok := tool.OutputSchema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("tool %q output schema properties missing", tool.Name)
		}
		errorSchema, ok := properties["error"].(map[string]any)
		if !ok {
			t.Fatalf("tool %q output schema missing error envelope", tool.Name)
		}
		errorProps, ok := errorSchema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("tool %q error envelope properties missing", tool.Name)
		}
		for _, field := range []string{"code", "message", "retryable"} {
			if _, ok := errorProps[field]; !ok {
				t.Fatalf("tool %q error envelope missing %q", tool.Name, field)
			}
		}
	}
}

func TestDiscoverTools_RepeatDiscoveryIsStable(t *testing.T) {
	t.Parallel()

	first, err := json.Marshal(factorydefinitionmcp.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal first discovery: %v", err)
	}
	second, err := json.Marshal(factorydefinitionmcp.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal second discovery: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeat discovery differs:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestToolByName_ReturnsCanonicalDefinitions(t *testing.T) {
	t.Parallel()

	for _, want := range factorydefinitionmcp.DiscoverTools() {
		got, ok := factorydefinitionmcp.ToolByName(want.Name)
		if !ok {
			t.Fatalf("ToolByName(%q) ok = false, want true", want.Name)
		}
		if got.Name != want.Name || got.Description != want.Description {
			t.Fatalf("ToolByName(%q) = %#v, want %#v", want.Name, got, want)
		}
	}

	if _, ok := factorydefinitionmcp.ToolByName("you.factory_definition.unknown"); ok {
		t.Fatal("ToolByName(unknown) ok = true, want false")
	}
}

func toolDefinitionsByName(t *testing.T) map[string]factorydefinitionmcp.ToolDefinition {
	t.Helper()

	byName := make(map[string]factorydefinitionmcp.ToolDefinition, len(factorydefinitionmcp.DiscoverTools()))
	for _, tool := range factorydefinitionmcp.DiscoverTools() {
		byName[tool.Name] = tool
	}
	return byName
}

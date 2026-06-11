package factorysession_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestDiscoverTools_ExposesExpectedFactorySessionTools(t *testing.T) {
	tools := mcpfactorysession.DiscoverTools()
	if len(tools) != 10 {
		t.Fatalf("tool count = %d, want 10", len(tools))
	}

	wantNames := []string{
		mcpfactorysession.ToolListSessions,
		mcpfactorysession.ToolValidateSource,
		mcpfactorysession.ToolStartSync,
		mcpfactorysession.ToolStartAsync,
		mcpfactorysession.ToolGetSession,
		mcpfactorysession.ToolGetResult,
		mcpfactorysession.ToolListDispatches,
		mcpfactorysession.ToolListArtifacts,
		mcpfactorysession.ToolControl,
		mcpfactorysession.ToolReadEvents,
	}
	gotNames := mcpfactorysession.ToolNames()
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("tool names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestDiscoverTools_EachToolHasSchemasDescriptionsAndStableFields(t *testing.T) {
	for _, tool := range mcpfactorysession.DiscoverTools() {
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

func TestDiscoverTools_UsesFactorySessionVocabularyNotWorkflowPreviewPrimarySurface(t *testing.T) {
	forbidden := []string{
		"/workflow-previews",
		"workflow-previews",
		"WorkflowPreview",
	}
	for _, tool := range mcpfactorysession.DiscoverTools() {
		encoded, err := json.Marshal(tool)
		if err != nil {
			t.Fatalf("marshal tool %q: %v", tool.Name, err)
		}
		payload := string(encoded)
		for _, term := range forbidden {
			if strings.Contains(payload, term) {
				t.Fatalf("tool %q exposes deprecated primary surface term %q", tool.Name, term)
			}
		}
		if !strings.Contains(tool.Description, "Factory Session") &&
			tool.Name != mcpfactorysession.ToolValidateSource {
			t.Fatalf("tool %q description should mention Factory Session vocabulary", tool.Name)
		}
	}
}

func TestDiscoverTools_OutputSchemasDocumentSharedErrorEnvelope(t *testing.T) {
	for _, tool := range mcpfactorysession.DiscoverTools() {
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

func TestDiscoverTools_RepresentativeSchemaFields(t *testing.T) {
	byName := map[string]mcpfactorysession.ToolDefinition{}
	for _, tool := range mcpfactorysession.DiscoverTools() {
		byName[tool.Name] = tool
	}

	listProps := byName[mcpfactorysession.ToolListSessions].InputSchema["properties"].(map[string]any)
	if _, ok := listProps["scope"]; !ok {
		t.Fatal("list sessions input missing scope")
	}

	validateProps := byName[mcpfactorysession.ToolValidateSource].InputSchema["properties"].(map[string]any)
	if _, ok := validateProps["sourceKind"]; !ok {
		t.Fatal("validate source input missing sourceKind")
	}

	startProps := byName[mcpfactorysession.ToolStartAsync].InputSchema["properties"].(map[string]any)
	if _, ok := startProps["requestId"]; !ok {
		t.Fatal("start async input missing requestId")
	}
	if _, ok := startProps["source"]; !ok {
		t.Fatal("start async input missing source")
	}

	getResultProps := byName[mcpfactorysession.ToolGetResult].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"sessionId", "mode", "includeArtifacts"} {
		if _, ok := getResultProps[field]; !ok {
			t.Fatalf("get result input missing %q", field)
		}
	}

	controlProps := byName[mcpfactorysession.ToolControl].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"sessionId", "operation"} {
		if _, ok := controlProps[field]; !ok {
			t.Fatalf("control input missing %q", field)
		}
	}

	eventProps := byName[mcpfactorysession.ToolReadEvents].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"sessionId", "afterEventId", "afterSequence"} {
		if _, ok := eventProps[field]; !ok {
			t.Fatalf("read events input missing %q", field)
		}
	}

	syncResultProps := byName[mcpfactorysession.ToolStartSync].OutputSchema["properties"].(map[string]any)["result"].(map[string]any)["properties"].(map[string]any)
	if _, ok := syncResultProps["syncOutcome"]; !ok {
		t.Fatal("start sync output result missing syncOutcome")
	}

	dispatchResultProps := byName[mcpfactorysession.ToolListDispatches].OutputSchema["properties"].(map[string]any)["result"].(map[string]any)["properties"].(map[string]any)
	dispatches, ok := dispatchResultProps["dispatches"].(map[string]any)
	if !ok {
		t.Fatal("list dispatches output missing dispatches array schema")
	}
	itemProps := dispatches["items"].(map[string]any)["properties"].(map[string]any)
	if _, ok := itemProps["dispatchId"]; !ok {
		t.Fatal("dispatch summary schema missing dispatchId")
	}
}

func TestDiscoverCompatibilityAliases_AreMarkedCompatibilityOnly(t *testing.T) {
	aliases := mcpfactorysession.DiscoverCompatibilityAliases()
	if len(aliases) == 0 {
		t.Fatal("expected workflow compatibility aliases")
	}
	for _, alias := range aliases {
		if !alias.CompatibilityOnly {
			t.Fatalf("alias %q must be compatibility-only", alias.Name)
		}
		if !strings.HasPrefix(alias.Name, "you.workflow.") {
			t.Fatalf("alias %q should use workflow vocabulary", alias.Name)
		}
		if !strings.HasPrefix(alias.CanonicalName, "you.factory_session.") {
			t.Fatalf("alias %q should resolve to canonical Factory Session tool, got %q", alias.Name, alias.CanonicalName)
		}
		if _, ok := mcpfactorysession.ToolByName(alias.CanonicalName); !ok {
			t.Fatalf("alias %q canonical target %q is not discoverable", alias.Name, alias.CanonicalName)
		}
		if !strings.Contains(alias.Description, "Compatibility-only") {
			t.Fatalf("alias %q description should document compatibility-only semantics", alias.Name)
		}
	}
}

func TestMockClientDiscovery_RoundTripsJSON(t *testing.T) {
	tools := mcpfactorysession.DiscoverTools()
	encoded, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("marshal discovery catalog: %v", err)
	}

	var decoded []mcpfactorysession.ToolDefinition
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal discovery catalog: %v", err)
	}
	if len(decoded) != len(tools) {
		t.Fatalf("decoded tool count = %d, want %d", len(decoded), len(tools))
	}
	if decoded[0].Name != mcpfactorysession.ToolListSessions {
		t.Fatalf("first tool = %q, want %q", decoded[0].Name, mcpfactorysession.ToolListSessions)
	}
}

func TestMockClient_WorkflowValidateAliasMatchesCanonicalBehavior(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "broken.js", "require('fs');")

	projectRootPtr := projectRoot
	sourceValue := "broken"
	request := factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRootPtr,
		SourceValue: &sourceValue,
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	client := mcpfactorysession.NewClient()
	canonicalRaw, err := client.CallTool(mcpfactorysession.ToolValidateSource, encoded)
	if err != nil {
		t.Fatalf("canonical validate: %v", err)
	}
	aliasRaw, err := client.CallTool(mcpfactorysession.ToolWorkflowValidate, encoded)
	if err != nil {
		t.Fatalf("alias validate: %v", err)
	}
	if string(canonicalRaw) != string(aliasRaw) {
		t.Fatalf("alias response = %s, want canonical %s", aliasRaw, canonicalRaw)
	}

	var response mcpfactorysession.ToolResponse[factoryapi.FactoryPreviewResult]
	if err := json.Unmarshal(aliasRaw, &response); err != nil {
		t.Fatalf("unmarshal alias response: %v", err)
	}
	if response.Result != nil || response.Error == nil {
		t.Fatalf("response = %#v, want typed validation error envelope", response)
	}
	if response.Error.Code == "" || response.Error.Message == "" {
		t.Fatalf("error envelope = %#v, want code and message", response.Error)
	}
}

func writeWorkflow(t *testing.T, projectRoot, name, content string) {
	t.Helper()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}

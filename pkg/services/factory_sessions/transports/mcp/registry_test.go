package factorysession_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

func TestDiscoverTools_MCPHostGuideTranscriptToolsAreRegistered(t *testing.T) {
	requiredByGuide := []string{
		"you.factory_session.validate_source",
		"you.factory_session.start_async",
		"you.factory_session.get",
		"you.factory_session.get_result",
		"you.factory_session.list_dispatches",
		"you.factory_session.list_artifacts",
		"you.factory_session.read_events",
		"you.factory_session.control",
	}
	registered := map[string]bool{}
	for _, tool := range mcpfactorysession.DiscoverTools() {
		registered[tool.Name] = true
	}

	for _, name := range requiredByGuide {
		if !registered[name] {
			t.Errorf("MCP host guide transcript tool %q is missing from the registered MCP catalog", name)
		}
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
	byName := toolDefinitionsByName(t)
	assertRepresentativeInputSchemaFields(t, byName)
	assertRepresentativeOutputSchemaFields(t, byName)
}

func assertRepresentativeInputSchemaFields(t *testing.T, byName map[string]mcpfactorysession.ToolDefinition) {
	t.Helper()
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

	getSessionProps := byName[mcpfactorysession.ToolGetSession].InputSchema["properties"].(map[string]any)
	if len(getSessionProps) != 1 {
		t.Fatalf("get session input fields = %#v, want only sessionId", getSessionProps)
	}
	dispatchProps := byName[mcpfactorysession.ToolListDispatches].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"sessionId", "phase", "status"} {
		if _, ok := dispatchProps[field]; !ok {
			t.Fatalf("list dispatches input missing %q", field)
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
}

func assertRepresentativeOutputSchemaFields(t *testing.T, byName map[string]mcpfactorysession.ToolDefinition) {
	t.Helper()
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
	if _, ok := itemProps["id"]; !ok {
		t.Fatal("dispatch summary schema missing id")
	}
	if _, ok := itemProps["dispatchKind"]; !ok {
		t.Fatal("dispatch summary schema missing dispatchKind")
	}
}

func TestDiscoverTools_ResponseSchemasMatchGeneratedAPITypes(t *testing.T) {
	byName := toolDefinitionsByName(t)

	t.Run("validate_source", func(t *testing.T) {
		assertValidateSourceSchemaMatchesGeneratedAPI(t, byName[mcpfactorysession.ToolValidateSource])
	})
	t.Run("get_session", func(t *testing.T) {
		assertGetSessionSchemaMatchesGeneratedAPI(t, byName[mcpfactorysession.ToolGetSession])
	})
	t.Run("get_result", func(t *testing.T) {
		assertGetResultSchemaMatchesGeneratedAPI(t, byName[mcpfactorysession.ToolGetResult])
	})
	t.Run("list_dispatches", func(t *testing.T) {
		assertListDispatchesSchemaMatchesGeneratedAPI(t, byName[mcpfactorysession.ToolListDispatches])
	})
	t.Run("list_artifacts", func(t *testing.T) {
		assertListArtifactsSchemaMatchesGeneratedAPI(t, byName[mcpfactorysession.ToolListArtifacts])
	})
	t.Run("read_events", func(t *testing.T) {
		assertReadEventsSchemaMatchesGeneratedAPI(t, byName[mcpfactorysession.ToolReadEvents])
	})
}

func toolDefinitionsByName(t *testing.T) map[string]mcpfactorysession.ToolDefinition {
	t.Helper()
	byName := map[string]mcpfactorysession.ToolDefinition{}
	for _, tool := range mcpfactorysession.DiscoverTools() {
		byName[tool.Name] = tool
	}
	return byName
}

func assertValidateSourceSchemaMatchesGeneratedAPI(t *testing.T, tool mcpfactorysession.ToolDefinition) {
	t.Helper()
	resultSchema := toolResultSchema(t, tool)
	requireSchemaFieldsMatchGenerated(
		t,
		resultSchema,
		reflect.TypeOf(factoryapi.FactoryPreviewResult{}),
		"valid",
		"sourceResolution",
		"policyPreview",
		"sourceValidationIssues",
	)
	policyPreview := nestedSchemaProperties(t, resultSchema, "policyPreview")
	requireSchemaFieldsMatchGenerated(
		t,
		policyPreview,
		reflect.TypeOf(factoryapi.WorkflowPolicyPreview{}),
		"policyHash",
	)
	requireSchemaFieldsAbsent(t, policyPreview, "effectivePolicyHash")
}

func assertGetSessionSchemaMatchesGeneratedAPI(t *testing.T, tool mcpfactorysession.ToolDefinition) {
	t.Helper()
	resultSchema := toolResultSchema(t, tool)
	requireSchemaFieldsMatchGenerated(
		t,
		resultSchema,
		reflect.TypeOf(factoryapi.FactorySessionDurableReadModel{}),
		"artifactRefs",
		"budgets",
		"effectivePolicy",
		"failureDetail",
		"latestCheckpoint",
		"lifecycle",
		"sessionId",
		"status",
		"orchestratorKind",
		"resolvedSource",
		"phaseSummaries",
		"progress",
		"resultSummary",
		"usage",
	)
	progress := nestedSchemaProperties(t, resultSchema, "progress")
	requireSchemaFieldsMatchGenerated(
		t,
		progress,
		reflect.TypeOf(factoryapi.FactorySessionDurableProgressCounts{}),
		"totalDispatches",
		"completedDispatches",
		"failedDispatches",
		"inFlightDispatches",
		"phaseCount",
	)
	requireSchemaFieldsAbsent(t, progress, "queued", "running", "succeeded", "failed")
}

func assertGetResultSchemaMatchesGeneratedAPI(t *testing.T, tool mcpfactorysession.ToolDefinition) {
	t.Helper()
	resultSchema := toolResultSchema(t, tool)
	requireSchemaFieldsMatchGenerated(
		t,
		resultSchema,
		reflect.TypeOf(factoryapi.FactorySessionResult{}),
		"sessionId",
		"resultStatus",
		"sessionStatus",
		"availability",
		"failureDetail",
	)
	availability := nestedSchemaProperties(t, resultSchema, "availability")
	requireSchemaFieldsMatchGenerated(
		t,
		availability,
		reflect.TypeOf(factoryapi.FactorySessionResultAvailabilityDetail{}),
		"reason",
		"message",
		"retryable",
	)
	requireSchemaFieldsAbsent(t, availability, "code")
	failure := nestedSchemaProperties(t, resultSchema, "failureDetail")
	requireSchemaFieldsMatchGenerated(
		t,
		failure,
		reflect.TypeOf(factoryapi.FailureDetail{}),
		"reason",
		"message",
	)
	requireSchemaFieldsAbsent(t, failure, "code")
}

func assertListDispatchesSchemaMatchesGeneratedAPI(t *testing.T, tool mcpfactorysession.ToolDefinition) {
	t.Helper()
	resultSchema := toolResultSchema(t, tool)
	dispatches := arrayItemSchemaProperties(t, resultSchema, "dispatches")
	requireSchemaFieldsMatchGenerated(
		t,
		dispatches,
		reflect.TypeOf(factoryapi.FactorySessionDispatchSummary{}),
		"id",
		"dispatchKind",
		"status",
		"phase",
		"label",
		"runnerId",
		"model",
		"providerSessionRefs",
		"attempt",
		"outputArtifactIds",
		"failureDetail",
	)
	requireSchemaFieldsAbsent(t, dispatches, "dispatchId", "kind", "sessionId")
}

func assertListArtifactsSchemaMatchesGeneratedAPI(t *testing.T, tool mcpfactorysession.ToolDefinition) {
	t.Helper()
	resultSchema := toolResultSchema(t, tool)
	artifacts := arrayItemSchemaProperties(t, resultSchema, "artifacts")
	requireSchemaFieldsMatchGenerated(
		t,
		artifacts,
		reflect.TypeOf(factoryapi.FactorySessionArtifactSummary{}),
		"id",
		"kind",
		"visibility",
		"contentHash",
		"dispatchId",
		"createdAt",
		"label",
		"auditMode",
		"redactionCounts",
		"retrievalRef",
	)
	requireSchemaFieldsAbsent(t, artifacts, "artifactId", "sessionId")
}

func assertReadEventsSchemaMatchesGeneratedAPI(t *testing.T, tool mcpfactorysession.ToolDefinition) {
	t.Helper()
	resultSchema := toolResultSchema(t, tool)
	events := arrayItemSchemaProperties(t, resultSchema, "events")
	requireSchemaFieldsMatchGenerated(
		t,
		events,
		reflect.TypeOf(factoryapi.FactoryEvent{}),
		"id",
		"type",
		"schemaVersion",
		"context",
		"payload",
	)
	requireSchemaFieldsAbsent(t, events, "timestamp")
	contextSchema := nestedSchemaProperties(t, events, "context")
	requireSchemaFieldsMatchGenerated(
		t,
		contextSchema,
		reflect.TypeOf(factoryapi.FactoryEventContext{}),
		"eventTime",
		"sequence",
		"sessionId",
		"sessionSequence",
	)
}

func toolResultSchema(t *testing.T, tool mcpfactorysession.ToolDefinition) map[string]any {
	t.Helper()
	properties, ok := tool.OutputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool %q output schema properties missing", tool.Name)
	}
	resultSchema, ok := properties["result"].(map[string]any)
	if !ok {
		t.Fatalf("tool %q output schema missing result envelope", tool.Name)
	}
	resultProps, ok := resultSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool %q result schema properties missing", tool.Name)
	}
	return resultProps
}

func nestedSchemaProperties(t *testing.T, schema map[string]any, field string) map[string]any {
	t.Helper()
	nested, ok := schema[field].(map[string]any)
	if !ok {
		t.Fatalf("schema missing nested field %q", field)
	}
	props, ok := nested["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema field %q missing properties", field)
	}
	return props
}

func arrayItemSchemaProperties(t *testing.T, schema map[string]any, field string) map[string]any {
	t.Helper()
	arraySchema, ok := schema[field].(map[string]any)
	if !ok {
		t.Fatalf("schema missing array field %q", field)
	}
	items, ok := arraySchema["items"].(map[string]any)
	if !ok {
		t.Fatalf("schema field %q missing items", field)
	}
	props, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema field %q items missing properties", field)
	}
	return props
}

func jsonFieldNames(typ reflect.Type) map[string]struct{} {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	names := make(map[string]struct{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			name = field.Name
		}
		names[name] = struct{}{}
	}
	return names
}

func requireSchemaFieldsMatchGenerated(
	t *testing.T,
	schema map[string]any,
	typ reflect.Type,
	fields ...string,
) {
	t.Helper()
	generated := jsonFieldNames(typ)
	for _, field := range fields {
		if _, ok := schema[field]; !ok {
			t.Fatalf("schema missing field %q", field)
		}
		if _, ok := generated[field]; !ok {
			t.Fatalf("generated type %s missing json field %q", typ.Name(), field)
		}
	}
}

func requireSchemaFieldsAbsent(t *testing.T, schema map[string]any, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if _, ok := schema[field]; ok {
			t.Fatalf("schema should not document stale field %q", field)
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

func writeWorkflow(t *testing.T, projectRoot, name, content string) {
	t.Helper()
	workflowDir := filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}

func TestProjectToolInventory_BuildsDocumentShape(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	if inventory.FormatVersion != mcpfactorysession.ToolInventoryFormatVersion {
		t.Fatalf("formatVersion = %q, want %q", inventory.FormatVersion, mcpfactorysession.ToolInventoryFormatVersion)
	}
	if inventory.ProtocolVersion != mcpfactorysession.ToolInventoryProtocolVersion {
		t.Fatalf("protocolVersion = %q, want %q", inventory.ProtocolVersion, mcpfactorysession.ToolInventoryProtocolVersion)
	}
	if len(inventory.Tools) != len(mcpfactorysession.DiscoverTools()) {
		t.Fatalf("tool count = %d, want %d", len(inventory.Tools), len(mcpfactorysession.DiscoverTools()))
	}
}

func TestProjectToolInventory_ToolsSortedByCanonicalName(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	names := make([]string, len(inventory.Tools))
	for i, tool := range inventory.Tools {
		names[i] = tool.Name
	}
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	if !slices.Equal(names, sorted) {
		t.Fatalf("tool names = %#v, want sorted %#v", names, sorted)
	}
}

func TestProjectToolInventory_EachToolHasIdentityFields(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	for _, tool := range inventory.Tools {
		if strings.TrimSpace(tool.IDCandidate) == "" {
			t.Fatalf("tool %q missing idCandidate", tool.Name)
		}
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
	}
}

func TestProjectToolInventory_DerivesStableIDCandidates(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	byName := inventoryToolsByName(t, inventory)
	if got := byName[mcpfactorysession.ToolGetSession].IDCandidate; got != "factory-session.get" {
		t.Fatalf("you.factory_session.get idCandidate = %q, want factory-session.get", got)
	}
	if got := byName[mcpfactorysession.ToolValidateSource].IDCandidate; got != "factory-session.validate-source" {
		t.Fatalf("you.factory_session.validate_source idCandidate = %q, want factory-session.validate-source", got)
	}
}

func TestProjectToolInventory_DoesNotAdvertiseResultContracts(t *testing.T) {
	encoded, err := mcpfactorysession.MarshalToolInventoryJSON(mustProjectToolInventory(t))
	if err != nil {
		t.Fatalf("MarshalToolInventoryJSON() error = %v", err)
	}
	payload := string(encoded)
	for _, forbidden := range []string{
		"outputSchema",
		"structuredContent",
		"successStableFields",
		"errorStableFields",
		"\"image\"",
		"\"audio\"",
		"\"resources\"",
		"\"task\"",
	} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("inventory advertises unsupported field %q", forbidden)
		}
	}
}

func TestProjectToolInventory_CanonicalizesNestedInputSchemaKeys(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	byName := inventoryToolsByName(t, inventory)
	startSync := byName[mcpfactorysession.ToolStartSync]
	properties, ok := startSync.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("start_sync input schema properties missing")
	}
	encoded, err := json.Marshal(properties)
	if err != nil {
		t.Fatalf("marshal start_sync properties: %v", err)
	}
	payload := string(encoded)
	wantPrefix := `{"args":`
	if !strings.HasPrefix(payload, wantPrefix) {
		t.Fatalf("marshaled properties = %s, want alphabetically sorted keys with %q first", payload, wantPrefix)
	}
}

func TestProjectToolInventory_DoesNotMutateDiscoverySchemas(t *testing.T) {
	before := cloneToolDefinitions(t, mcpfactorysession.DiscoverTools())
	if _, err := mcpfactorysession.ProjectToolInventory(); err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	after := mcpfactorysession.DiscoverTools()
	if len(before) != len(after) {
		t.Fatalf("discover tool count changed: before=%d after=%d", len(before), len(after))
	}
	for i := range before {
		beforeJSON, err := json.Marshal(before[i].InputSchema)
		if err != nil {
			t.Fatalf("marshal before schema: %v", err)
		}
		afterJSON, err := json.Marshal(after[i].InputSchema)
		if err != nil {
			t.Fatalf("marshal after schema: %v", err)
		}
		if string(beforeJSON) != string(afterJSON) {
			t.Fatalf("tool %q input schema mutated by inventory projection", before[i].Name)
		}
	}
}

func TestProjectToolInventory_RepeatExtractionIsByteIdentical(t *testing.T) {
	first, err := mcpfactorysession.MarshalToolInventoryJSON(mustProjectToolInventory(t))
	if err != nil {
		t.Fatalf("first MarshalToolInventoryJSON() error = %v", err)
	}
	second, err := mcpfactorysession.MarshalToolInventoryJSON(mustProjectToolInventory(t))
	if err != nil {
		t.Fatalf("second MarshalToolInventoryJSON() error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeat extraction differs:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestProjectToolInventory_MatchesDiscoverToolsIdentityFields(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	byName := inventoryToolsByName(t, inventory)
	for _, discovered := range mcpfactorysession.DiscoverTools() {
		entry, ok := byName[discovered.Name]
		if !ok {
			t.Fatalf("inventory missing discovered tool %q", discovered.Name)
		}
		if entry.Description != discovered.Description {
			t.Fatalf("tool %q description = %q, want %q", discovered.Name, entry.Description, discovered.Description)
		}
		canonicalSchema, err := json.Marshal(entry.InputSchema)
		if err != nil {
			t.Fatalf("marshal inventory schema for %q: %v", discovered.Name, err)
		}
		sourceSchema, err := json.Marshal(discovered.InputSchema)
		if err != nil {
			t.Fatalf("marshal discovered schema for %q: %v", discovered.Name, err)
		}
		if string(canonicalSchema) != string(sourceSchema) {
			t.Fatalf("tool %q input schema differs after canonicalization:\ninventory=%s\ndiscovered=%s", discovered.Name, canonicalSchema, sourceSchema)
		}
	}
}

func TestProjectToolInventory_HandlerRegisteredForCanonicalTools(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	for _, tool := range inventory.Tools {
		if !tool.HandlerRegistered {
			t.Fatalf("tool %q handlerRegistered = false, want true", tool.Name)
		}
		if !mcpfactorysession.IsCanonicalToolHandlerRegistered(tool.Name) {
			t.Fatalf("tool %q is not registered in the canonical handler registry", tool.Name)
		}
	}
}

func TestVerifyProjectedToolInventory_PassesForLiveRegistry(t *testing.T) {
	if err := mcpfactorysession.VerifyProjectedToolInventory(); err != nil {
		t.Fatalf("VerifyProjectedToolInventory() error = %v", err)
	}
}

func TestBaselineFixtureMatchesProjectedInventory(t *testing.T) {
	baselinePath := testutil.MustRepoPath(t, mcpfactorysession.ToolInventoryBaselineRelativePath)
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline fixture: %v", err)
	}
	projected, err := mcpfactorysession.MarshalToolInventoryJSON(mustProjectToolInventory(t))
	if err != nil {
		t.Fatalf("MarshalToolInventoryJSON() error = %v", err)
	}
	if string(baseline) != string(projected) {
		t.Fatalf("baseline fixture differs from projected inventory:\nbaseline=%s\nprojected=%s", baseline, projected)
	}
}

func TestBaselineFixtureMatchesDiscoverToolsRegistry(t *testing.T) {
	baselinePath := testutil.MustRepoPath(t, mcpfactorysession.ToolInventoryBaselineRelativePath)
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline fixture: %v", err)
	}
	var inventory mcpfactorysession.ToolInventory
	if err := json.Unmarshal(baseline, &inventory); err != nil {
		t.Fatalf("unmarshal baseline fixture: %v", err)
	}
	if inventory.FormatVersion != mcpfactorysession.ToolInventoryFormatVersion {
		t.Fatalf("baseline formatVersion = %q, want %q", inventory.FormatVersion, mcpfactorysession.ToolInventoryFormatVersion)
	}
	if inventory.ProtocolVersion != mcpfactorysession.ToolInventoryProtocolVersion {
		t.Fatalf("baseline protocolVersion = %q, want %q", inventory.ProtocolVersion, mcpfactorysession.ToolInventoryProtocolVersion)
	}
	if len(inventory.Tools) != len(mcpfactorysession.DiscoverTools()) {
		t.Fatalf("baseline tool count = %d, want %d", len(inventory.Tools), len(mcpfactorysession.DiscoverTools()))
	}
	if err := mcpfactorysession.VerifyToolInventory(inventory); err != nil {
		t.Fatalf("VerifyToolInventory(baseline) error = %v", err)
	}
	byName := inventoryToolsByName(t, inventory)
	for _, discovered := range mcpfactorysession.DiscoverTools() {
		entry, ok := byName[discovered.Name]
		if !ok {
			t.Fatalf("baseline missing discovered tool %q", discovered.Name)
		}
		if entry.Description != discovered.Description {
			t.Fatalf("baseline tool %q description = %q, want %q", discovered.Name, entry.Description, discovered.Description)
		}
		canonicalSchema, err := json.Marshal(entry.InputSchema)
		if err != nil {
			t.Fatalf("marshal baseline schema for %q: %v", discovered.Name, err)
		}
		sourceSchema, err := json.Marshal(discovered.InputSchema)
		if err != nil {
			t.Fatalf("marshal discovered schema for %q: %v", discovered.Name, err)
		}
		if string(canonicalSchema) != string(sourceSchema) {
			t.Fatalf("baseline tool %q input schema differs from discovery:\nbaseline=%s\ndiscovered=%s", discovered.Name, canonicalSchema, sourceSchema)
		}
	}
}

func TestVerifyToolInventory_FailsWhenDiscoveredToolMissingHandler(t *testing.T) {
	const unregisteredTool = "you.factory_session.unregistered_probe"
	discovered := []mcpfactorysession.ToolDefinition{{
		Name:        unregisteredTool,
		Description: "probe tool without handler registration",
		InputSchema: map[string]any{"type": "object"},
	}}
	inventory, err := mcpfactorysession.ProjectToolInventoryFromDiscovered(discovered)
	if err != nil {
		t.Fatalf("ProjectToolInventoryFromDiscovered() error = %v", err)
	}
	if inventory.Tools[0].HandlerRegistered {
		t.Fatalf("tool %q handlerRegistered = true, want false", unregisteredTool)
	}
	err = mcpfactorysession.VerifyToolInventory(inventory)
	if err == nil {
		t.Fatal("VerifyToolInventory() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), unregisteredTool) {
		t.Fatalf("VerifyToolInventory() error = %v, want offending tool %q", err, unregisteredTool)
	}
}

func mustProjectToolInventory(t *testing.T) mcpfactorysession.ToolInventory {
	t.Helper()
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	return inventory
}

func inventoryToolsByName(t *testing.T, inventory mcpfactorysession.ToolInventory) map[string]mcpfactorysession.ToolInventoryEntry {
	t.Helper()
	byName := make(map[string]mcpfactorysession.ToolInventoryEntry, len(inventory.Tools))
	for _, tool := range inventory.Tools {
		byName[tool.Name] = tool
	}
	return byName
}

func cloneToolDefinitions(t *testing.T, tools []mcpfactorysession.ToolDefinition) []mcpfactorysession.ToolDefinition {
	t.Helper()
	encoded, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("marshal tool definitions: %v", err)
	}
	var cloned []mcpfactorysession.ToolDefinition
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		t.Fatalf("unmarshal tool definitions: %v", err)
	}
	return cloned
}

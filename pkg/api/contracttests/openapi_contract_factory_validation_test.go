package apicontract_test

import (
	"os"
	"strings"
	"testing"
)

const compatibilityRegisterPath = "../../../docs/internal/processes/api-relevant-files.md"

type compatibilityAliasExpectation struct {
	alias     string
	successor string
}

func TestOpenAPIContract_DefinesFactoryValidationEndpoint(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths object is missing")
	}

	pathItem, ok := paths["/factory-validations"].(map[string]any)
	if !ok {
		t.Fatal("paths./factory-validations is missing")
	}
	postOperation, ok := pathItem["post"].(map[string]any)
	if !ok {
		t.Fatal("paths./factory-validations.post is missing")
	}
	if got, _ := postOperation["operationId"].(string); got != "validateFactory" {
		t.Fatalf("paths./factory-validations.post.operationId = %q, want validateFactory", got)
	}
	assertRequestSchemaRef(t, postOperation, "#/components/schemas/Factory")
	assertResponseSchemaRef(t, postOperation, "200", "#/components/schemas/FactoryValidationResult")
	assertResponseRef(t, postOperation, "400", "#/components/responses/BadRequest")
}

func TestOpenAPIContract_FactoryValidationResultSchemaMatchesCanonicalTargetShape(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	doc := loadValidatedOpenAPIContract(t)

	resultSchema := schemaObject(t, schemas, "FactoryValidationResult")
	assertRequiredFields(t, resultSchema, "targets")
	resultProperties := schemaProperties(t, resultSchema, "FactoryValidationResult")
	assertArrayItemRef(t, resultProperties, "targets", "#/components/schemas/FactoryValidationTarget")

	targetSchema := schemaObject(t, schemas, "FactoryValidationTarget")
	assertRequiredFields(t, targetSchema, "code", "severity", "message", "subject")
	targetProperties := schemaProperties(t, targetSchema, "FactoryValidationTarget")
	assertPropertyRef(t, targetProperties, "severity", "#/components/schemas/FactoryValidationSeverity")
	assertPropertyRef(t, targetProperties, "subject", "#/components/schemas/FactoryValidationSubject")

	subjectSchema := schemaObject(t, schemas, "FactoryValidationSubject")
	assertRequiredFields(t, subjectSchema, "type", "id", "location")
	subjectProperties := schemaProperties(t, subjectSchema, "FactoryValidationSubject")
	assertPropertyRef(t, subjectProperties, "type", "#/components/schemas/FactoryValidationSubjectType")
	assertPropertyRef(t, subjectProperties, "location", "#/components/schemas/FactoryValidationSubjectLocation")

	assertEnumValues(t, schemaObject(t, schemas, "FactoryValidationSeverity"), "FactoryValidationSeverity", []string{"error", "warning", "hint"})
	assertEnumValues(t, schemaObject(t, schemas, "FactoryValidationSubjectType"), "FactoryValidationSubjectType", []string{
		"FACTORY", "WORKSTATION", "WORK_TYPE", "WORK_STATE", "WORKER", "RESOURCE", "ROUTE",
	})
	assertEnumValues(t, schemaObject(t, schemas, "FactoryValidationSubjectLocation"), "FactoryValidationSubjectLocation", []string{
		"ON_REJECTION", "ON_FAILURE", "OUTPUTS", "INPUTS", "STATES", "TERMINAL", "REFERENCE", "DEFINITION",
	})

	resultOpenAPI := requireOpenAPI3ComponentSchema(t, doc, "FactoryValidationResult")
	if resultOpenAPI.Example == nil {
		t.Fatal("FactoryValidationResult.example is missing")
	}
	if err := resultOpenAPI.VisitJSON(resultOpenAPI.Example); err != nil {
		t.Fatalf("FactoryValidationResult.example should validate: %v", err)
	}
	exampleTargets, ok := resultOpenAPI.Example.(map[string]any)["targets"].([]any)
	if !ok || len(exampleTargets) < 2 {
		t.Fatalf("FactoryValidationResult.example.targets = %#v, want multi-target invalid factory example", resultOpenAPI.Example)
	}
}

func TestOpenAPIContract_DefinesFactoryPreviewEndpoint(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths object is missing")
	}

	pathItem, ok := paths["/factories/preview"].(map[string]any)
	if !ok {
		t.Fatal("paths./factories/preview is missing")
	}
	postOperation, ok := pathItem["post"].(map[string]any)
	if !ok {
		t.Fatal("paths./factories/preview.post is missing")
	}
	if got, _ := postOperation["operationId"].(string); got != "previewFactory" {
		t.Fatalf("paths./factories/preview.post.operationId = %q, want previewFactory", got)
	}
	if deprecated, ok := postOperation["deprecated"].(bool); ok && deprecated {
		t.Fatal("paths./factories/preview.post must not be deprecated")
	}
	tags, ok := postOperation["tags"].([]any)
	if !ok || len(tags) == 0 {
		t.Fatal("paths./factories/preview.post.tags is missing")
	}
	if got, _ := tags[0].(string); got != "Factory" {
		t.Fatalf("paths./factories/preview.post.tags[0] = %q, want Factory", got)
	}
	assertRequestSchemaRef(t, postOperation, "#/components/schemas/FactoryPreviewRequest")
	assertResponseSchemaRef(t, postOperation, "200", "#/components/schemas/FactoryPreviewResult")
	assertResponseRef(t, postOperation, "400", "#/components/responses/BadRequest")
}

func TestOpenAPIContract_DefinesWorkflowPreviewCompatibilityOnlyEndpoint(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths object is missing")
	}

	pathItem, ok := paths["/workflow-previews"].(map[string]any)
	if !ok {
		t.Fatal("paths./workflow-previews is missing")
	}
	postOperation, ok := pathItem["post"].(map[string]any)
	if !ok {
		t.Fatal("paths./workflow-previews.post is missing")
	}
	if deprecated, _ := postOperation["deprecated"].(bool); !deprecated {
		t.Fatal("paths./workflow-previews.post.deprecated should be true")
	}
	if got, _ := postOperation["operationId"].(string); got != "previewWorkflow" {
		t.Fatalf("paths./workflow-previews.post.operationId = %q, want previewWorkflow", got)
	}
	description, _ := postOperation["description"].(string)
	if !strings.Contains(description, "/factories/preview") {
		t.Fatalf("paths./workflow-previews.post.description = %q, want successor reference to /factories/preview", description)
	}
	assertRequestSchemaRef(t, postOperation, "#/components/schemas/WorkflowPreviewRequest")
	assertResponseSchemaRef(t, postOperation, "200", "#/components/schemas/WorkflowPreviewResult")
	assertResponseRef(t, postOperation, "400", "#/components/responses/BadRequest")
}

func TestOpenAPIContract_FactoryPreviewRequestSchemaMatchesSharedContract(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	requestSchema := schemaObject(t, schemas, "FactoryPreviewRequest")
	if deprecated, ok := requestSchema["deprecated"].(bool); ok && deprecated {
		t.Fatal("FactoryPreviewRequest must not be deprecated")
	}
	assertRequiredFields(t, requestSchema, "sourceKind")
	requestProperties := schemaProperties(t, requestSchema, "FactoryPreviewRequest")
	sourceKind, ok := requestProperties["sourceKind"].(map[string]any)
	if !ok {
		t.Fatal("FactoryPreviewRequest.properties.sourceKind is missing")
	}
	assertEnumValues(t, sourceKind, "FactoryPreviewRequest.properties.sourceKind", []string{
		"FACTORY_ID",
		"FACTORY_INLINE",
		"WORKFLOW_FILE",
		"WORKFLOW_NAME",
		"INLINE_WORKFLOW",
	})
}

func TestOpenAPIContract_FactoryPreviewResultSchemaMatchesSharedContract(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	resultSchema := schemaObject(t, schemas, "FactoryPreviewResult")
	if deprecated, ok := resultSchema["deprecated"].(bool); ok && deprecated {
		t.Fatal("FactoryPreviewResult must not be deprecated")
	}
	assertRequiredFields(t, resultSchema,
		"valid",
		"sourceResolution",
		"sourceValidationIssues",
		"policyPreview",
		"resultConstraints",
	)
	resultProperties := schemaProperties(t, resultSchema, "FactoryPreviewResult")
	assertPropertyRef(t, resultProperties, "sourceResolution", "#/components/schemas/WorkflowSourceResolution")
	assertPropertyRef(t, resultProperties, "policyPreview", "#/components/schemas/WorkflowPolicyPreview")
	assertPropertyRef(t, resultProperties, "resultConstraints", "#/components/schemas/WorkflowResultConstraints")
}

func TestOpenAPIContract_WorkflowPreviewSchemasAreCompatibilityOnlyAliasesOfFactoryPreview(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	workflowRequest := schemaObject(t, schemas, "WorkflowPreviewRequest")
	if deprecated, _ := workflowRequest["deprecated"].(bool); !deprecated {
		t.Fatal("WorkflowPreviewRequest should be deprecated")
	}
	workflowResult := schemaObject(t, schemas, "WorkflowPreviewResult")
	if deprecated, _ := workflowResult["deprecated"].(bool); !deprecated {
		t.Fatal("WorkflowPreviewResult should be deprecated")
	}
}

func TestCompatibilityAliasRegister_WorkflowAliasesHaveCompleteMeasurableRecords(t *testing.T) {
	data, err := os.ReadFile(compatibilityRegisterPath)
	if err != nil {
		t.Fatalf("read compatibility register: %v", err)
	}
	register := string(data)
	expectations := []compatibilityAliasExpectation{
		{alias: "`POST /workflow-previews` (`previewWorkflow`)", successor: "`POST /factories/preview` (`previewFactory`)"},
		{alias: "`WorkflowPreviewRequest`, `WorkflowPreviewResult`", successor: "`FactoryPreviewRequest`, `FactoryPreviewResult`"},
		{alias: "`you workflow preview`", successor: "`POST /factories/preview`"},
		{alias: "`you workflow validate`", successor: "`POST /factories/preview` or MCP `you.factory_session.validate_source`"},
		{alias: "`you workflow run`", successor: "`POST /factory-sessions/sync`"},
		{alias: "`you workflow start`", successor: "`POST /factory-sessions/async`"},
		{alias: "`you workflow status {session_id}`", successor: "`GET /factory-sessions/{session_id}`"},
		{alias: "`you workflow result {session_id}`", successor: "`GET /factory-sessions/{session_id}/results`"},
		{alias: "`you workflow dispatches {session_id}`", successor: "`GET /factory-sessions/{session_id}/dispatches`"},
		{alias: "`you workflow artifacts {session_id}`", successor: "`GET /factory-sessions/{session_id}/artifacts`"},
		{alias: "`you workflow events {session_id}`", successor: "`GET /factory-sessions/{session_id}/events`"},
		{alias: "MCP `you.workflow.validate`", successor: "MCP `you.factory_session.validate_source`"},
		{alias: "MCP `you.workflow.run`", successor: "MCP `you.factory_session.start_sync`"},
		{alias: "MCP `you.workflow.status`", successor: "MCP `you.factory_session.get`"},
		{alias: "MCP `you.workflow.result`", successor: "MCP `you.factory_session.get_result`"},
		{alias: "MCP `you.workflow.artifacts`", successor: "MCP `you.factory_session.list_artifacts`"},
		{alias: "UI `WorkflowPreviewRequest`, `WorkflowPreviewResult`", successor: "`FactoryPreviewRequest`, `FactoryPreviewResult`"},
	}
	rows := compatibilityRegisterRows(register)
	if len(rows) != len(expectations) {
		t.Fatalf("workflow compatibility register has %d entries, want %d", len(rows), len(expectations))
	}
	for _, expectation := range expectations {
		row, ok := findCompatibilityRow(rows, expectation.alias)
		if !ok {
			t.Errorf("compatibility register is missing alias %s", expectation.alias)
			continue
		}
		for column, value := range row {
			if strings.TrimSpace(value) == "" {
				t.Errorf("alias %s has an empty %s field", expectation.alias, column)
			}
		}
		if !strings.Contains(row["successor"], expectation.successor) {
			t.Errorf("alias %s successor %q does not resolve to %s", expectation.alias, row["successor"], expectation.successor)
		}
		if strings.Contains(strings.ToLower(strings.Join(rowValues(row), " ")), "workflow-run resource") {
			t.Errorf("alias %s is presented as an independent workflow-run resource", expectation.alias)
		}
	}
	assertMeasurableRemovalPolicy(t, register, "### Factory preview API and CLI")
	assertMeasurableRemovalPolicy(t, register, "### MCP and dashboard workflow-named wrappers")
	if strings.Contains(register, "recorded as removal-approved") || strings.Contains(register, "approved for removal: yes") {
		t.Fatal("a retained public alias is recorded as approved for removal")
	}
}

func compatibilityRegisterRows(register string) []map[string]string {
	var rows []map[string]string
	for _, line := range strings.Split(register, "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Compatibility alias") {
			continue
		}
		columns := strings.Split(strings.Trim(line, "|"), "|")
		if len(columns) != 6 || strings.TrimSpace(columns[0]) == "---" {
			continue
		}
		alias := strings.TrimSpace(columns[0])
		if !strings.Contains(alias, "workflow") && !strings.Contains(alias, "Workflow") {
			continue
		}
		rows = append(rows, map[string]string{"alias": alias, "boundary": strings.TrimSpace(columns[1]), "successor": strings.TrimSpace(columns[2]), "owner": strings.TrimSpace(columns[3]), "compatibility": strings.TrimSpace(columns[4]), "removal gate": strings.TrimSpace(columns[5])})
	}
	return rows
}

func findCompatibilityRow(rows []map[string]string, alias string) (map[string]string, bool) {
	for _, row := range rows {
		if strings.Contains(row["alias"], alias) {
			return row, true
		}
	}
	return nil, false
}

func rowValues(row map[string]string) []string {
	values := make([]string, 0, len(row))
	for _, value := range row {
		values = append(values, value)
	}
	return values
}

func assertMeasurableRemovalPolicy(t *testing.T, register, heading string) {
	t.Helper()
	section := strings.Join(strings.Fields(markdownSection(register, heading)), " ")
	for _, phrase := range []string{"next major release", "consecutive minor releases", "support", "zero", "No entry below is recorded as satisfying this gate or approved for removal."} {
		if !strings.Contains(section, phrase) {
			t.Errorf("%s removal policy is missing measurable requirement %q", heading, phrase)
		}
	}
	if !strings.Contains(section, "evidence source") && !strings.Contains(section, "telemetry") {
		t.Errorf("%s removal policy does not name an evidence source", heading)
	}
}

func markdownSection(markdown, heading string) string {
	start := strings.Index(markdown, heading)
	if start < 0 {
		return ""
	}
	section := markdown[start+len(heading):]
	if end := strings.Index(section, "\n### "); end >= 0 {
		section = section[:end]
	}
	return section
}

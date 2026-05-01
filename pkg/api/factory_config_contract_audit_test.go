package api

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"gopkg.in/yaml.v3"
)

const (
	factoryConfigOpenAPIRootSchema = "Factory"
	factoryConfigSchemaRefPrefix   = "#/components/schemas/"
	factoryConfigCamelCaseRule     = "public Agent Factory factory-config fields must use camelCase; keep legacy aliases boundary-only"
)

var targetedFactoryConfigSnakeCaseFields = []string{
	"ExhaustionRule.max_visits",
	"ExhaustionRule.watch_workstation",
	"Factory.exhaustion_rules",
	"Factory.factory_dir",
	"Factory.input_types",
	"Factory.source_directory",
	"Factory.work_types",
	"Factory.workflow_id",
	"InputGuard.parent_input",
	"InputGuard.spawned_by",
	"Worker.model_provider",
	"Worker.session_id",
	"Worker.skip_permissions",
	"Worker.stop_token",
	"Workstation.on_failure",
	"Workstation.on_rejection",
	"Workstation.output_schema",
	"Workstation.prompt_file",
	"Workstation.prompt_template",
	"Workstation.copy_referenced_scripts",
	"Workstation.resourceUsage",
	"Workstation.resource_usage",
	"Workstation.runtimeStopWords",
	"Workstation.runtime_stop_words",
	"Workstation.stopToken",
	"Workstation.stop_words",
	"Workstation.working_directory",
	"WorkstationCron.expiry_window",
	"WorkstationCron.trigger_at_start",
	"WorkstationGuard.max_visits",
	"WorkstationIO.work_type",
	"WorkstationLimits.max_execution_time",
	"WorkstationLimits.max_retries",
}

func TestFactoryConfigContractAudit_TargetedSnakeCaseFieldsAreAbsentFromOpenAPIAndGeneratedModels(t *testing.T) {
	openAPIFields := collectFactoryConfigOpenAPIFields(t)
	generatedFields := collectFactoryConfigGeneratedJSONFields()

	for _, field := range targetedFactoryConfigSnakeCaseFields {
		assertFieldAbsent(t, "openapi", openAPIFields, field)
		assertFieldAbsent(t, "generated", generatedFields, "generated."+field)
	}
}

func TestFactoryConfigContractGuard_PublicOpenAPIFactoryConfigFieldsUseCamelCase(t *testing.T) {
	offenses := snakeCaseFactoryConfigFields(collectFactoryConfigOpenAPIFields(t))
	if len(offenses) == 0 {
		return
	}

	t.Fatalf("%s in OpenAPI:\n- %s", factoryConfigCamelCaseRule, strings.Join(offenses, "\n- "))
}

func TestFactoryConfigContractGuard_PublicGeneratedFactoryConfigJSONTagsUseCamelCase(t *testing.T) {
	offenses := snakeCaseFactoryConfigFields(collectFactoryConfigGeneratedJSONFields())
	if len(offenses) == 0 {
		return
	}

	t.Fatalf("%s in generated models:\n- %s", factoryConfigCamelCaseRule, strings.Join(offenses, "\n- "))
}

func TestFactoryConfigContractAudit_RegressionOpenAPICollectorDetectsSnakeCaseNestedField(t *testing.T) {
	doc := map[string]any{
		"components": map[string]any{
			"schemas": map[string]any{
				"Factory": map[string]any{
					"properties": map[string]any{
						"metadata": map[string]any{
							"type": "object",
							"additionalProperties": map[string]any{
								"type": "string",
							},
						},
						"workstations": map[string]any{
							"type": "array",
							"items": map[string]any{
								"$ref": "#/components/schemas/Workstation",
							},
						},
					},
				},
				"Workstation": map[string]any{
					"properties": map[string]any{
						"cron": map[string]any{
							"$ref": "#/components/schemas/WorkstationCron",
						},
					},
				},
				"WorkstationCron": map[string]any{
					"properties": map[string]any{
						"trigger_at_start": map[string]any{
							"type": "boolean",
						},
					},
				},
			},
		},
	}

	offenses := snakeCaseFactoryConfigFields(collectFactoryConfigOpenAPIFieldsFromDocument(t, doc, factoryConfigOpenAPIRootSchema))
	want := []string{"WorkstationCron.trigger_at_start"}
	if !reflect.DeepEqual(offenses, want) {
		t.Fatalf("expected OpenAPI regression audit to catch nested snake_case fields, got %#v", offenses)
	}
}

func TestFactoryConfigContractAudit_RegressionGeneratedCollectorDetectsSnakeCaseNestedJSONTags(t *testing.T) {
	rootType := reflect.TypeOf(syntheticFactoryConfigAuditFactory{})
	offenses := snakeCaseFactoryConfigFields(
		collectFactoryConfigGeneratedJSONFieldsFromRoot(rootType, "generated."+rootType.Name()),
	)
	want := []string{
		"generated.syntheticFactoryConfigAuditCron.trigger_at_start",
		"generated.syntheticFactoryConfigAuditGuard.spawned_by",
	}
	if !reflect.DeepEqual(offenses, want) {
		t.Fatalf("expected generated-model regression audit to catch nested snake_case json tags, got %#v", offenses)
	}
}

func collectFactoryConfigOpenAPIFields(t *testing.T) map[string]struct{} {
	t.Helper()

	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}

	return collectFactoryConfigOpenAPIFieldsFromDocument(t, doc, factoryConfigOpenAPIRootSchema)
}

func collectFactoryConfigOpenAPIFieldsFromDocument(
	t *testing.T,
	doc map[string]any,
	rootSchema string,
) map[string]struct{} {
	t.Helper()

	schemas := componentSchemas(t, doc)
	fields := make(map[string]struct{})
	visited := make(map[string]bool)
	collectFactoryConfigOpenAPIComponentFields(t, schemas, rootSchema, visited, fields)

	return fields
}

func collectFactoryConfigGeneratedJSONFields() map[string]struct{} {
	rootType := reflect.TypeOf(factoryapi.Factory{})
	return collectFactoryConfigGeneratedJSONFieldsFromRoot(rootType, "generated."+rootType.Name())
}

func collectFactoryConfigGeneratedJSONFieldsFromRoot(rootType reflect.Type, rootName string) map[string]struct{} {
	fields := make(map[string]struct{})
	visited := make(map[reflect.Type]bool)
	collectFactoryConfigGeneratedTypeFields(rootType.PkgPath(), rootType, rootName, visited, fields)

	return fields
}

func collectFactoryConfigOpenAPIComponentFields(
	t *testing.T,
	schemas map[string]any,
	schemaName string,
	visited map[string]bool,
	fields map[string]struct{},
) {
	t.Helper()

	if visited[schemaName] {
		return
	}
	visited[schemaName] = true

	collectFactoryConfigOpenAPISchemaFields(t, schemas, schemaName, schemaObject(t, schemas, schemaName), visited, fields)
}

func collectFactoryConfigOpenAPISchemaFields(
	t *testing.T,
	schemas map[string]any,
	path string,
	schema map[string]any,
	visited map[string]bool,
	fields map[string]struct{},
) {
	t.Helper()

	properties, ok := schema["properties"].(map[string]any)
	if ok {
		for propertyName, propertyAny := range properties {
			propertyPath := path + "." + propertyName
			fields[propertyPath] = struct{}{}

			propertySchema, ok := propertyAny.(map[string]any)
			if !ok {
				continue
			}
			collectFactoryConfigOpenAPISubSchemaFields(t, schemas, propertyPath, propertySchema, visited, fields)
		}
	}

	if additionalProperties, ok := schema["additionalProperties"].(map[string]any); ok {
		collectFactoryConfigOpenAPISubSchemaFields(t, schemas, path, additionalProperties, visited, fields)
	}
}

func collectFactoryConfigOpenAPISubSchemaFields(
	t *testing.T,
	schemas map[string]any,
	path string,
	schema map[string]any,
	visited map[string]bool,
	fields map[string]struct{},
) {
	t.Helper()

	if refName, ok := openAPISchemaNameFromRef(schema["$ref"]); ok {
		collectFactoryConfigOpenAPIComponentFields(t, schemas, refName, visited, fields)
	}
	if items, ok := schema["items"].(map[string]any); ok {
		collectFactoryConfigOpenAPISubSchemaFields(t, schemas, path, items, visited, fields)
	}
	for _, compositionKey := range []string{"allOf", "anyOf", "oneOf"} {
		items, ok := schema[compositionKey].([]any)
		if !ok {
			continue
		}
		for _, item := range items {
			itemSchema, ok := item.(map[string]any)
			if !ok {
				continue
			}
			collectFactoryConfigOpenAPISubSchemaFields(t, schemas, path, itemSchema, visited, fields)
		}
	}
	if _, ok := schema["properties"].(map[string]any); ok {
		collectFactoryConfigOpenAPISchemaFields(t, schemas, path, schema, visited, fields)
	}
	if additionalProperties, ok := schema["additionalProperties"].(map[string]any); ok {
		collectFactoryConfigOpenAPISubSchemaFields(t, schemas, path, additionalProperties, visited, fields)
	}
}

func collectFactoryConfigGeneratedTypeFields(
	generatedPackagePath string,
	typ reflect.Type,
	typeName string,
	visited map[reflect.Type]bool,
	fields map[string]struct{},
) {
	typ = dereferencedType(typ)
	if typ.Kind() != reflect.Struct || typ.PkgPath() != generatedPackagePath || visited[typ] {
		return
	}
	visited[typ] = true

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			fields[typeName+"."+name] = struct{}{}
		}
		collectFactoryConfigGeneratedNestedTypeFields(generatedPackagePath, field.Type, visited, fields)
	}
}

func collectFactoryConfigGeneratedNestedTypeFields(
	generatedPackagePath string,
	typ reflect.Type,
	visited map[reflect.Type]bool,
	fields map[string]struct{},
) {
	typ = dereferencedType(typ)

	switch typ.Kind() {
	case reflect.Array, reflect.Slice:
		collectFactoryConfigGeneratedNestedTypeFields(generatedPackagePath, typ.Elem(), visited, fields)
	case reflect.Map:
		collectFactoryConfigGeneratedNestedTypeFields(generatedPackagePath, typ.Elem(), visited, fields)
	case reflect.Struct:
		if typ.PkgPath() != generatedPackagePath || typ.Name() == "" {
			return
		}
		collectFactoryConfigGeneratedTypeFields(generatedPackagePath, typ, "generated."+typ.Name(), visited, fields)
	}
}

func dereferencedType(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

func openAPISchemaNameFromRef(ref any) (string, bool) {
	refString, ok := ref.(string)
	if !ok || !strings.HasPrefix(refString, factoryConfigSchemaRefPrefix) {
		return "", false
	}
	return strings.TrimPrefix(refString, factoryConfigSchemaRefPrefix), true
}

func snakeCaseFactoryConfigFields(fields map[string]struct{}) []string {
	var offenses []string
	for fieldPath := range fields {
		if !strings.Contains(factoryConfigLeafFieldName(fieldPath), "_") {
			continue
		}
		offenses = append(offenses, fieldPath)
	}
	sort.Strings(offenses)
	return offenses
}

func factoryConfigLeafFieldName(fieldPath string) string {
	segments := strings.Split(fieldPath, ".")
	if len(segments) == 0 {
		return fieldPath
	}
	return segments[len(segments)-1]
}

func factoryConfigTokenPattern(token string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(token) + `($|[^A-Za-z0-9_])`)
}

func assertFieldAbsent(t *testing.T, surface string, fields map[string]struct{}, name string) {
	t.Helper()

	if _, ok := fields[name]; ok {
		t.Fatalf("%s field %s must not be advertised after the camelCase contract migration", surface, name)
	}
}

type syntheticFactoryConfigAuditFactory struct {
	Metadata     map[string]string               `json:"metadata"`
	Workstations []syntheticFactoryConfigAuditWS `json:"workstations"`
}

type syntheticFactoryConfigAuditWS struct {
	Cron   syntheticFactoryConfigAuditCron    `json:"cron"`
	Guards []syntheticFactoryConfigAuditGuard `json:"guards"`
}

type syntheticFactoryConfigAuditCron struct {
	TriggerAtStart bool `json:"trigger_at_start"`
}

type syntheticFactoryConfigAuditGuard struct {
	Env       map[string]string `json:"env"`
	SpawnedBy string            `json:"spawned_by"`
}

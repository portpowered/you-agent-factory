package api

import (
	"os"
	"reflect"
	"testing"

	"github.com/portpowered/agent-factory/pkg/api/generated"
	"gopkg.in/yaml.v3"
)

func TestFactoryConfigContract_OpenAPIEnumBackedFieldsReferenceNamedSchemas(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}

	schemas := componentSchemas(t, doc)
	assertSchemaPropertyRef(t, schemas, "InputType", "type", "#/components/schemas/InputKind")
	assertSchemaPropertyRef(t, schemas, "WorkState", "type", "#/components/schemas/WorkStateType")
	assertSchemaPropertyRef(t, schemas, "Worker", "type", "#/components/schemas/WorkerType")
	assertSchemaPropertyRef(t, schemas, "Worker", "modelProvider", "#/components/schemas/WorkerModelProvider")
	assertSchemaPropertyRef(t, schemas, "Worker", "executorProvider", "#/components/schemas/WorkerProvider")
	assertSchemaPropertyRef(t, schemas, "Workstation", "kind", "#/components/schemas/WorkstationKind")
	assertSchemaPropertyRef(t, schemas, "Workstation", "type", "#/components/schemas/WorkstationType")
	assertSchemaPropertyRef(t, schemas, "WorkstationGuard", "type", "#/components/schemas/WorkstationGuardType")
	assertSchemaPropertyRef(t, schemas, "InputGuard", "type", "#/components/schemas/InputGuardType")
}

func TestFactoryConfigContract_GeneratedModelsUseEnumBackedFieldsForTightenedConfigFields(t *testing.T) {
	assertGeneratedFieldType(t, reflect.TypeOf(generated.InputType{}), "Type", reflect.TypeOf(generated.InputKind("")))
	assertGeneratedFieldType(t, reflect.TypeOf(generated.WorkState{}), "Type", reflect.TypeOf(generated.WorkStateType("")))
	assertGeneratedFieldType(t, reflect.TypeOf(generated.Worker{}), "Type", reflect.TypeOf((*generated.WorkerType)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(generated.Worker{}), "ModelProvider", reflect.TypeOf((*generated.WorkerModelProvider)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(generated.Worker{}), "ExecutorProvider", reflect.TypeOf((*generated.WorkerProvider)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(generated.Workstation{}), "Kind", reflect.TypeOf((*generated.WorkstationKind)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(generated.Workstation{}), "Type", reflect.TypeOf((*generated.WorkstationType)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(generated.WorkstationGuard{}), "Type", reflect.TypeOf(generated.WorkstationGuardType("")))
	assertGeneratedFieldType(t, reflect.TypeOf(generated.InputGuard{}), "Type", reflect.TypeOf(generated.InputGuardType("")))
}

func TestFactoryConfigContract_CanonicalPayloadExercisesGeneratedEnumBackedFields(t *testing.T) {
	factory := decodeGeneratedFactoryForSmoke(t, []byte(factoryConfigSmokeCanonicalJSON()))

	if factory.InputTypes == nil || len(*factory.InputTypes) != 1 {
		t.Fatalf("canonical factory inputTypes = %#v, want one enum-backed input type", factory.InputTypes)
	}
	if (*factory.InputTypes)[0].Type != generated.InputKindDefault {
		t.Fatalf("canonical factory input type = %q, want DEFAULT", (*factory.InputTypes)[0].Type)
	}

	if factory.WorkTypes == nil || len(*factory.WorkTypes) != 2 {
		t.Fatalf("canonical factory workTypes = %#v, want parent/story work types", factory.WorkTypes)
	}
	states := (*factory.WorkTypes)[1].States
	if len(states) != 3 {
		t.Fatalf("canonical work type states = %#v, want init/failed/complete", states)
	}
	if states[0].Type != generated.WorkStateTypeINITIAL || states[1].Type != generated.WorkStateTypeFAILED || states[2].Type != generated.WorkStateTypeTERMINAL {
		t.Fatalf("canonical work state types = %#v, want INITIAL/FAILED/TERMINAL", []generated.WorkStateType{states[0].Type, states[1].Type, states[2].Type})
	}

	if factory.Workers == nil || len(*factory.Workers) != 1 {
		t.Fatalf("canonical factory workers = %#v, want one worker", factory.Workers)
	}
	worker := (*factory.Workers)[0]
	if worker.Type == nil || *worker.Type != generated.WorkerTypeModelWorker {
		t.Fatalf("canonical worker type = %#v, want MODEL_WORKER", worker.Type)
	}
	if worker.ModelProvider == nil || *worker.ModelProvider != generated.WorkerModelProviderClaude {
		t.Fatalf("canonical worker modelProvider = %#v, want CLAUDE", worker.ModelProvider)
	}
	if worker.ExecutorProvider == nil || *worker.ExecutorProvider != generated.WorkerProviderScriptWrap {
		t.Fatalf("canonical worker executorProvider = %#v, want SCRIPT_WRAP", worker.ExecutorProvider)
	}

	if factory.Workstations == nil || len(*factory.Workstations) != 3 {
		t.Fatalf("canonical factory workstations = %#v, want execute-story/fanout/guard-cycle", factory.Workstations)
	}

	executeStory := (*factory.Workstations)[0]
	if executeStory.Kind == nil || *executeStory.Kind != generated.WorkstationKindCron {
		t.Fatalf("canonical workstation kind = %#v, want CRON", executeStory.Kind)
	}
	if executeStory.Type == nil || *executeStory.Type != generated.WorkstationTypeModelWorkstation {
		t.Fatalf("canonical workstation type = %#v, want MODEL_WORKSTATION", executeStory.Type)
	}
	if executeStory.Guards == nil || len(*executeStory.Guards) != 1 || (*executeStory.Guards)[0].Type != generated.WorkstationGuardTypeVisitCount {
		t.Fatalf("canonical workstation guards = %#v, want one VISIT_COUNT guard", executeStory.Guards)
	}
	if len(executeStory.Inputs) < 2 || executeStory.Inputs[1].Guards == nil || len(*executeStory.Inputs[1].Guards) != 1 || (*executeStory.Inputs[1].Guards)[0].Type != generated.InputGuardTypeAllChildrenComplete {
		t.Fatalf("canonical workstation input guards = %#v, want ALL_CHILDREN_COMPLETE", executeStory.Inputs)
	}

	guardCycle := (*factory.Workstations)[2]
	if guardCycle.Type == nil || *guardCycle.Type != generated.WorkstationTypeLogicalMove {
		t.Fatalf("canonical loop-breaker type = %#v, want LOGICAL_MOVE", guardCycle.Type)
	}
}

func assertSchemaPropertyRef(t *testing.T, schemas map[string]any, schemaName string, propertyName string, wantRef string) {
	t.Helper()

	assertPropertyRef(t, schemaProperties(t, schemaObject(t, schemas, schemaName), schemaName), propertyName, wantRef)
}

func assertGeneratedFieldType(t *testing.T, structType reflect.Type, fieldName string, wantType reflect.Type) {
	t.Helper()

	field, ok := structType.FieldByName(fieldName)
	if !ok {
		t.Fatalf("%s.%s is missing", structType.Name(), fieldName)
	}
	if field.Type != wantType {
		t.Fatalf("%s.%s type = %s, want %s", structType.Name(), fieldName, field.Type, wantType)
	}
}

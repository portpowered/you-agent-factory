package factorydefinition_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionmcp "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mcp"
)

const minimalValidationFactoryBody = `{
  "name": "alpha",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "done", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{
    "name": "planner",
    "type": "MODEL_WORKER",
    "modelProvider": "CLAUDE",
    "executorProvider": "SCRIPT_WRAP",
    "model": "claude-sonnet-4-20250514"
  }],
  "workstations": [{
    "name": "plan-task",
    "behavior": "STANDARD",
    "type": "MODEL_WORKSTATION",
    "worker": "planner",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "done"}]
  }]
}`

func TestBind_FakeValidationInvokedThroughValidateTool(t *testing.T) {
	t.Parallel()

	validation := &mcpDefinitionsValidationFake{}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Validation: validation})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolValidate,
		json.RawMessage(minimalValidationFactoryBody),
	)
	if err != nil {
		t.Fatalf("CallTool(validate) error = %v", err)
	}
	if !validation.invoked {
		t.Fatal("ValidateSubmittedDefinition was not invoked through the injected validation operation")
	}
	if !strings.Contains(string(raw), `"targets"`) {
		t.Fatalf("CallTool(validate) = %s, want encoded validation result", raw)
	}
}

func TestBind_FakeDefinitionsRootInvokedThroughValidateTool(t *testing.T) {
	t.Parallel()

	root := &mcpDefinitionsRootFake{}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Definitions: root})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolValidate,
		json.RawMessage(minimalValidationFactoryBody),
	)
	if err != nil {
		t.Fatalf("CallTool(validate) error = %v", err)
	}
	if !root.validateStructuralInvoked {
		t.Fatal("ValidateStructuralFactoryDefinition was not invoked through the injected Definitions root")
	}
	if !strings.Contains(string(raw), `"targets"`) {
		t.Fatalf("CallTool(validate) = %s, want encoded validation result", raw)
	}
}

func TestBind_UnsupportedToolReturnsStableError(t *testing.T) {
	t.Parallel()

	operation := factorydefinitionmcp.NewFromRoot(factorydefinitionmcp.RootBinding{})
	_, err := operation(context.Background(), "you.factory_definition.unknown", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("CallTool(unknown) error = nil, want unsupported tool error")
	}
	if !strings.Contains(err.Error(), "unsupported tool") {
		t.Fatalf("CallTool(unknown) error = %v, want unsupported tool message", err)
	}
}

type mcpDefinitionsValidationFake struct {
	invoked bool
}

func (fake *mcpDefinitionsValidationFake) ValidateSubmittedDefinition(
	_ context.Context,
	_ factorydefinitions.SubmittedDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	fake.invoked = true
	return factorydefinitions.ValidationResult{
		Targets: []factorydefinitions.ValidationTarget{{
			Code:     "factory.validation.stub",
			Severity: factorydefinitions.ValidationSeverityError,
			Message:  "stub validation finding",
			Subject: factorydefinitions.ValidationSubject{
				Type:     factorydefinitions.ValidationSubjectTypeFactory,
				Location: factorydefinitions.ValidationSubjectLocationDefinition,
			},
		}},
	}, nil
}

type mcpDefinitionsRootFake struct {
	validateStructuralInvoked bool
}

var _ factorydefinitions.Service = (*mcpDefinitionsRootFake)(nil)

func (fake *mcpDefinitionsRootFake) ValidateStructuralFactoryDefinition(
	_ context.Context,
	request factorydefinitions.ValidateStructuralFactoryDefinitionRequest,
) (factorydefinitions.ValidateStructuralFactoryDefinitionResult, error) {
	fake.validateStructuralInvoked = true
	if len(request.Canonical) == 0 {
		return factorydefinitions.ValidateStructuralFactoryDefinitionResult{}, factorydefinitions.ErrInvalidFactoryDefinitionPayload
	}
	return factorydefinitions.ValidateStructuralFactoryDefinitionResult{
		Validation: factorydefinitions.ValidationResult{},
	}, nil
}

func (fake *mcpDefinitionsRootFake) ActivateNamedFactory(context.Context, string) error { return nil }

func (fake *mcpDefinitionsRootFake) Save(
	context.Context,
	string,
	factorydefinitions.SaveMode,
	factorydefinitions.EditableFactory,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, nil
}

func (fake *mcpDefinitionsRootFake) GetCurrentNamedFactory(context.Context) (*factorydefinitions.FactorySnapshot, error) {
	return nil, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fake *mcpDefinitionsRootFake) GetCurrentFactoryForSession(
	context.Context,
	string,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fake *mcpDefinitionsRootFake) CurrentFactoryDefinitionVersionAtRoot(
	string,
	string,
) (factorydefinitions.FactoryVersion, error) {
	return factorydefinitions.FactoryVersion{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fake *mcpDefinitionsRootFake) ListEffectiveFactories(
	context.Context,
	factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	return factorydefinitions.ListEffectiveFactoriesResult{}, nil
}

func (fake *mcpDefinitionsRootFake) ListNamedFactories(
	context.Context,
	factorydefinitions.ListNamedFactoriesRequest,
) (factorydefinitions.ListNamedFactoriesResult, error) {
	return factorydefinitions.ListNamedFactoriesResult{}, nil
}

func (fake *mcpDefinitionsRootFake) GetNamedFactory(
	context.Context,
	factorydefinitions.GetNamedFactoryRequest,
) (factorydefinitions.GetNamedFactoryResult, error) {
	return factorydefinitions.GetNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
}

func (fake *mcpDefinitionsRootFake) ResolveNamedFactory(
	context.Context,
	factorydefinitions.ResolveNamedFactoryRequest,
) (factorydefinitions.ResolveNamedFactoryResult, error) {
	return factorydefinitions.ResolveNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
}

func (fake *mcpDefinitionsRootFake) DeleteNamedFactory(
	context.Context,
	factorydefinitions.DeleteNamedFactoryRequest,
) (factorydefinitions.DeleteNamedFactoryResult, error) {
	return factorydefinitions.DeleteNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
}

func (fake *mcpDefinitionsRootFake) GetCurrentFactoryPointer(
	context.Context,
	factorydefinitions.GetCurrentFactoryPointerRequest,
) (factorydefinitions.GetCurrentFactoryPointerResult, error) {
	return factorydefinitions.GetCurrentFactoryPointerResult{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fake *mcpDefinitionsRootFake) SetCurrentFactoryPointer(
	context.Context,
	factorydefinitions.SetCurrentFactoryPointerRequest,
) (factorydefinitions.SetCurrentFactoryPointerResult, error) {
	return factorydefinitions.SetCurrentFactoryPointerResult{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fake *mcpDefinitionsRootFake) PrepareFactoryLayout(
	context.Context,
	factorydefinitions.PrepareFactoryLayoutRequest,
) (factorydefinitions.PrepareFactoryLayoutResult, error) {
	return factorydefinitions.PrepareFactoryLayoutResult{}, factorydefinitions.ErrMalformedFactoryLayoutPayload
}

func (fake *mcpDefinitionsRootFake) FlattenFactoryLayout(
	context.Context,
	factorydefinitions.FlattenFactoryLayoutRequest,
) (factorydefinitions.FlattenFactoryLayoutResult, error) {
	return factorydefinitions.FlattenFactoryLayoutResult{}, factorydefinitions.ErrMalformedFactoryLayoutPayload
}

func (fake *mcpDefinitionsRootFake) ExpandFactoryLayout(
	context.Context,
	factorydefinitions.ExpandFactoryLayoutRequest,
) (factorydefinitions.ExpandFactoryLayoutResult, error) {
	return factorydefinitions.ExpandFactoryLayoutResult{}, factorydefinitions.ErrMalformedFactoryLayoutPayload
}

func (fake *mcpDefinitionsRootFake) CreateNamedFactory(
	context.Context,
	factorydefinitions.CreateNamedFactoryRequest,
) (factorydefinitions.CreateNamedFactoryResult, error) {
	return factorydefinitions.CreateNamedFactoryResult{}, factorydefinitions.ErrAtomicFactoryWriteFailed
}

func (fake *mcpDefinitionsRootFake) ReplaceNamedFactory(
	context.Context,
	factorydefinitions.ReplaceNamedFactoryRequest,
) (factorydefinitions.ReplaceNamedFactoryResult, error) {
	return factorydefinitions.ReplaceNamedFactoryResult{}, factorydefinitions.ErrAtomicFactoryWriteFailed
}

func (fake *mcpDefinitionsRootFake) CompileEffectiveFactorySource(
	context.Context,
	factorydefinitions.CompileEffectiveFactorySourceRequest,
) (factorydefinitions.CompileEffectiveFactorySourceResult, error) {
	return factorydefinitions.CompileEffectiveFactorySourceResult{}, factorydefinitions.ErrInvalidAuthoredFactorySource
}

func (fake *mcpDefinitionsRootFake) ValidateEffectiveFactoryDefinition(
	context.Context,
	factorydefinitions.ValidateEffectiveFactoryDefinitionRequest,
) (factorydefinitions.ValidateEffectiveFactoryDefinitionResult, error) {
	return factorydefinitions.ValidateEffectiveFactoryDefinitionResult{}, factorydefinitions.ErrInvalidFactoryDefinitionPayload
}

func (fake *mcpDefinitionsRootFake) CaptureFactorySnapshot(
	context.Context,
	factorydefinitions.CaptureFactorySnapshotRequest,
) (factorydefinitions.CaptureFactorySnapshotResult, error) {
	return factorydefinitions.CaptureFactorySnapshotResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
}

func (fake *mcpDefinitionsRootFake) PrepareFactorySnapshotImport(
	context.Context,
	factorydefinitions.PrepareFactorySnapshotImportRequest,
) (factorydefinitions.PrepareFactorySnapshotImportResult, error) {
	return factorydefinitions.PrepareFactorySnapshotImportResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
}

func (fake *mcpDefinitionsRootFake) MaterializeFactorySnapshot(
	context.Context,
	factorydefinitions.MaterializeFactorySnapshotRequest,
) (factorydefinitions.MaterializeFactorySnapshotResult, error) {
	return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
}

func (fake *mcpDefinitionsRootFake) ListBuiltInPackagedFactories(
	context.Context,
	factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, nil
}

func (fake *mcpDefinitionsRootFake) ResolveBuiltInPackagedFactory(
	context.Context,
	factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
	return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, factorydefinitions.ErrUnknownPackagedFactoryIdentity
}

func (fake *mcpDefinitionsRootFake) InstallPackagedFactory(
	context.Context,
	factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
	return factorydefinitions.InstallPackagedFactoryResult{}, factorydefinitions.ErrFactoryDistributeFailed
}

func (fake *mcpDefinitionsRootFake) CreateFactoryScaffold(
	context.Context,
	factorydefinitions.CreateFactoryScaffoldRequest,
) (factorydefinitions.CreateFactoryScaffoldResult, error) {
	return factorydefinitions.CreateFactoryScaffoldResult{}, factorydefinitions.ErrFactoryDistributeFailed
}

var _ factorydefinitions.SubmittedDefinitionValidationOperation = (*mcpDefinitionsValidationFake)(nil)

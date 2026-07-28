package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionshttp "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/http"
	"go.uber.org/zap"
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

func TestHandlerFromRoot_ValidateFactoryInvokesSubmittedDefinitionValidationOperation(t *testing.T) {
	t.Parallel()

	validation := &httpDefinitionsValidationFake{}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Validation: validation},
		zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		strings.NewReader(minimalValidationFactoryBody),
	)
	request.Header.Set("Content-Type", "application/json")

	handler.ValidateFactory(recorder, request)

	if !validation.invoked {
		t.Fatal("ValidateSubmittedDefinition was not invoked through the injected validation operation")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("response = %d %s, want 200 from fake validation operation", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"targets"`) {
		t.Fatalf("response = %s, want encoded validation result", recorder.Body.String())
	}
}

func TestHandlerFromRoot_ValidateFactoryInvokesDefinitionsRootValidateStructural(t *testing.T) {
	t.Parallel()

	root := &httpDefinitionsRootFake{}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Definitions: root},
		zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		strings.NewReader(minimalValidationFactoryBody),
	)
	request.Header.Set("Content-Type", "application/json")

	handler.ValidateFactory(recorder, request)

	if !root.validateStructuralInvoked {
		t.Fatal("ValidateStructuralFactoryDefinition was not invoked through the injected Definitions root")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("response = %d %s, want 200 from fake Definitions root", recorder.Code, recorder.Body.String())
	}
}

type httpDefinitionsValidationFake struct {
	invoked bool
}

func (fake *httpDefinitionsValidationFake) ValidateSubmittedDefinition(
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

type httpDefinitionsRootFake struct {
	validateStructuralInvoked bool
}

var _ factorydefinitions.Service = (*httpDefinitionsRootFake)(nil)

func (fake *httpDefinitionsRootFake) ValidateStructuralFactoryDefinition(
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

func (fake *httpDefinitionsRootFake) ActivateNamedFactory(context.Context, string) error { return nil }

func (fake *httpDefinitionsRootFake) Save(
	context.Context,
	string,
	factorydefinitions.SaveMode,
	factorydefinitions.EditableFactory,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, nil
}

func (fake *httpDefinitionsRootFake) GetCurrentNamedFactory(context.Context) (*factorydefinitions.FactorySnapshot, error) {
	return nil, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fake *httpDefinitionsRootFake) GetCurrentFactoryForSession(
	context.Context,
	string,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fake *httpDefinitionsRootFake) CurrentFactoryDefinitionVersionAtRoot(
	string,
	string,
) (factorydefinitions.FactoryVersion, error) {
	return factorydefinitions.FactoryVersion{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fake *httpDefinitionsRootFake) ListEffectiveFactories(
	context.Context,
	factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	return factorydefinitions.ListEffectiveFactoriesResult{}, nil
}

func (fake *httpDefinitionsRootFake) ListNamedFactories(
	context.Context,
	factorydefinitions.ListNamedFactoriesRequest,
) (factorydefinitions.ListNamedFactoriesResult, error) {
	return factorydefinitions.ListNamedFactoriesResult{}, nil
}

func (fake *httpDefinitionsRootFake) GetNamedFactory(
	context.Context,
	factorydefinitions.GetNamedFactoryRequest,
) (factorydefinitions.GetNamedFactoryResult, error) {
	return factorydefinitions.GetNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
}

func (fake *httpDefinitionsRootFake) ResolveNamedFactory(
	context.Context,
	factorydefinitions.ResolveNamedFactoryRequest,
) (factorydefinitions.ResolveNamedFactoryResult, error) {
	return factorydefinitions.ResolveNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
}

func (fake *httpDefinitionsRootFake) DeleteNamedFactory(
	context.Context,
	factorydefinitions.DeleteNamedFactoryRequest,
) (factorydefinitions.DeleteNamedFactoryResult, error) {
	return factorydefinitions.DeleteNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
}

func (fake *httpDefinitionsRootFake) GetCurrentFactoryPointer(
	context.Context,
	factorydefinitions.GetCurrentFactoryPointerRequest,
) (factorydefinitions.GetCurrentFactoryPointerResult, error) {
	return factorydefinitions.GetCurrentFactoryPointerResult{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fake *httpDefinitionsRootFake) SetCurrentFactoryPointer(
	context.Context,
	factorydefinitions.SetCurrentFactoryPointerRequest,
) (factorydefinitions.SetCurrentFactoryPointerResult, error) {
	return factorydefinitions.SetCurrentFactoryPointerResult{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fake *httpDefinitionsRootFake) PrepareFactoryLayout(
	context.Context,
	factorydefinitions.PrepareFactoryLayoutRequest,
) (factorydefinitions.PrepareFactoryLayoutResult, error) {
	return factorydefinitions.PrepareFactoryLayoutResult{}, factorydefinitions.ErrMalformedFactoryLayoutPayload
}

func (fake *httpDefinitionsRootFake) FlattenFactoryLayout(
	context.Context,
	factorydefinitions.FlattenFactoryLayoutRequest,
) (factorydefinitions.FlattenFactoryLayoutResult, error) {
	return factorydefinitions.FlattenFactoryLayoutResult{}, factorydefinitions.ErrMalformedFactoryLayoutPayload
}

func (fake *httpDefinitionsRootFake) ExpandFactoryLayout(
	context.Context,
	factorydefinitions.ExpandFactoryLayoutRequest,
) (factorydefinitions.ExpandFactoryLayoutResult, error) {
	return factorydefinitions.ExpandFactoryLayoutResult{}, factorydefinitions.ErrMalformedFactoryLayoutPayload
}

func (fake *httpDefinitionsRootFake) CreateNamedFactory(
	context.Context,
	factorydefinitions.CreateNamedFactoryRequest,
) (factorydefinitions.CreateNamedFactoryResult, error) {
	return factorydefinitions.CreateNamedFactoryResult{}, factorydefinitions.ErrAtomicFactoryWriteFailed
}

func (fake *httpDefinitionsRootFake) ReplaceNamedFactory(
	context.Context,
	factorydefinitions.ReplaceNamedFactoryRequest,
) (factorydefinitions.ReplaceNamedFactoryResult, error) {
	return factorydefinitions.ReplaceNamedFactoryResult{}, factorydefinitions.ErrAtomicFactoryWriteFailed
}

func (fake *httpDefinitionsRootFake) CompileEffectiveFactorySource(
	context.Context,
	factorydefinitions.CompileEffectiveFactorySourceRequest,
) (factorydefinitions.CompileEffectiveFactorySourceResult, error) {
	return factorydefinitions.CompileEffectiveFactorySourceResult{}, factorydefinitions.ErrInvalidAuthoredFactorySource
}

func (fake *httpDefinitionsRootFake) ValidateEffectiveFactoryDefinition(
	context.Context,
	factorydefinitions.ValidateEffectiveFactoryDefinitionRequest,
) (factorydefinitions.ValidateEffectiveFactoryDefinitionResult, error) {
	return factorydefinitions.ValidateEffectiveFactoryDefinitionResult{}, factorydefinitions.ErrInvalidFactoryDefinitionPayload
}

func (fake *httpDefinitionsRootFake) CaptureFactorySnapshot(
	context.Context,
	factorydefinitions.CaptureFactorySnapshotRequest,
) (factorydefinitions.CaptureFactorySnapshotResult, error) {
	return factorydefinitions.CaptureFactorySnapshotResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
}

func (fake *httpDefinitionsRootFake) PrepareFactorySnapshotImport(
	context.Context,
	factorydefinitions.PrepareFactorySnapshotImportRequest,
) (factorydefinitions.PrepareFactorySnapshotImportResult, error) {
	return factorydefinitions.PrepareFactorySnapshotImportResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
}

func (fake *httpDefinitionsRootFake) MaterializeFactorySnapshot(
	context.Context,
	factorydefinitions.MaterializeFactorySnapshotRequest,
) (factorydefinitions.MaterializeFactorySnapshotResult, error) {
	return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
}

func (fake *httpDefinitionsRootFake) ListBuiltInPackagedFactories(
	context.Context,
	factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, nil
}

func (fake *httpDefinitionsRootFake) ResolveBuiltInPackagedFactory(
	context.Context,
	factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
	return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, factorydefinitions.ErrUnknownPackagedFactoryIdentity
}

func (fake *httpDefinitionsRootFake) InstallPackagedFactory(
	context.Context,
	factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
	return factorydefinitions.InstallPackagedFactoryResult{}, factorydefinitions.ErrFactoryDistributeFailed
}

func (fake *httpDefinitionsRootFake) CreateFactoryScaffold(
	context.Context,
	factorydefinitions.CreateFactoryScaffoldRequest,
) (factorydefinitions.CreateFactoryScaffoldResult, error) {
	return factorydefinitions.CreateFactoryScaffoldResult{}, factorydefinitions.ErrFactoryDistributeFailed
}

// Ensure the fake validation operation satisfies the root-adjacent contract.
var _ factorydefinitions.SubmittedDefinitionValidationOperation = (*httpDefinitionsValidationFake)(nil)

// Ensure generated handler signature compatibility for future route registration.
var _ interface {
	ValidateFactory(http.ResponseWriter, *http.Request)
} = (*factorydefinitionshttp.Adapter)(nil)

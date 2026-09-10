package cli

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestRootInvokeMapsUnsupportedOperationToBadRequest(t *testing.T) {
	t.Parallel()

	root := &rootService{models: &preflightModelsRoot{
		getCatalogErr: modelinference.ErrUnsupportedOperation,
	}}
	_, err := root.catalogForInvoke(
		InvokeConfig{Context: context.Background(), JSON: true},
		preflightTestScope(t), "llm", "NOPE",
	)
	assertCLIContractBadRequest(t, err, "unknown operation \"NOPE\"")
}

func TestRootInvokePreflightsGenericContractBeforeAssetEstimate(t *testing.T) {
	t.Parallel()

	operation, ok := (modelinference.GenericOperationCatalog{}).GenericOperationContract(modelinference.OperationOMNI)
	if !ok {
		t.Fatal("generic OMNI operation is not published")
	}
	cases := []struct {
		name       string
		config     InvokeConfig
		wantClass  modelinference.InvocationFailureClass
		wantPhrase string
	}{
		{
			name: "unknown slot", config: InvokeConfig{
				Context: context.Background(), Output: io.Discard,
				InputMappings: []string{"missing=value"}, JSON: true,
			}, wantClass: modelinference.InvocationFailureClassInvalidSlot, wantPhrase: "unknown input slot",
		},
		{
			name: "missing required slot", config: InvokeConfig{
				Context: context.Background(), Output: io.Discard,
				InputMappings: []string{`parameters=json:{"temperature":0.2}`}, JSON: true,
			}, wantClass: modelinference.InvocationFailureClassInvalidSlot, wantPhrase: "required input slot is missing",
		},
		{
			name: "duplicate slot", config: InvokeConfig{
				Context: context.Background(), Output: io.Discard,
				InputSpecs: []string{
					`{"name":"prompt","modality":"TEXT","content":"one"}`,
					`{"name":"prompt","modality":"TEXT","content":"two"}`,
				}, JSON: true,
			}, wantClass: modelinference.InvocationFailureClassSlotArity, wantPhrase: "accepts at most one value",
		},
		{
			name: "malformed parameter", config: InvokeConfig{
				Context: context.Background(), Output: io.Discard,
				ParameterSpecs: []string{`{"name":"temperature","value":}`}, JSON: true,
			}, wantClass: modelinference.InvocationFailureClassInvalidParameter, wantPhrase: "parse --parameter 1",
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			scope := preflightTestScope(t)
			events := []string{}
			modelsRoot := &preflightModelsRoot{
				catalog: modelinference.Detail{Summary: modelinference.Summary{
					Name: "llm", Operations: []modelinference.Operation{operation},
				}}, events: &events,
			}
			root := &rootService{models: modelsRoot}
			cfg := testCase.config
			_, err := root.invokeGenericInScope(cfg, scope, "llm", modelinference.OperationOMNI, "", modelsRoot.catalog)
			if err == nil {
				t.Fatal("invokeGenericInScope returned nil, want typed preflight failure")
			}
			var failure *modelinference.InvocationFailure
			if !errors.As(err, &failure) || failure == nil || failure.Class != testCase.wantClass {
				t.Fatalf("invokeGenericInScope error = %v, want invocation class %q", err, testCase.wantClass)
			}
			assertCLIContractBadRequest(t, err, testCase.wantPhrase)
			if len(events) != 0 {
				t.Fatalf("preflight effects = %#v, want no asset estimate or invocation", events)
			}
		})
	}
}

func TestRootInvokeMapsInvalidOutputMappingsToBadRequest(t *testing.T) {
	t.Parallel()

	operation, ok := (modelinference.GenericOperationCatalog{}).GenericOperationContract(modelinference.OperationOMNI)
	if !ok {
		t.Fatal("generic OMNI operation is not published")
	}
	cases := []struct {
		name       string
		mappings   []string
		wantPhrase string
	}{
		{name: "unknown output slot", mappings: []string{"text=result.txt", "other=other.txt"}, wantPhrase: "unknown slot"},
		{name: "incomplete output mapping", mappings: []string{"text=result.txt"}, wantPhrase: "cover every output slot"},
		{name: "malformed output mapping", mappings: []string{"text"}, wantPhrase: "expected slot=path"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := validateCLIOutputShape(
				InvokeConfig{JSON: true, OutputMappings: testCase.mappings},
				modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{operation}}},
				modelinference.OperationOMNI,
			)
			assertCLIContractBadRequest(t, mapModelsClientError(err), testCase.wantPhrase)
		})
	}
}

func assertCLIContractBadRequest(t *testing.T, err error, wantMessage string) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want BAD_REQUEST")
	}
	var coded interface {
		CLIErrorCode() string
		CLIErrorFamily() factoryapi.ErrorFamily
		CLIErrorMessage() string
	}
	if !errors.As(err, &coded) {
		t.Fatalf("error = %v, want coded CLI diagnostic", err)
	}
	if coded.CLIErrorCode() != modelsRootBadRequestCode || coded.CLIErrorFamily() != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("CLI diagnostic = (%q, %q), want BAD_REQUEST/BAD_REQUEST", coded.CLIErrorCode(), coded.CLIErrorFamily())
	}
	if !strings.Contains(coded.CLIErrorMessage(), wantMessage) {
		t.Fatalf("CLI diagnostic message = %q, want %q", coded.CLIErrorMessage(), wantMessage)
	}
	if strings.Contains(coded.CLIErrorMessage(), "CLI_COMMAND_FAILED") || strings.Contains(coded.CLIErrorMessage(), "INTERNAL_SERVER_ERROR") {
		t.Fatalf("CLI diagnostic fell back to generic failure: %q", coded.CLIErrorMessage())
	}
}

type preflightModelsRoot struct {
	modelinference.Service
	catalog       modelinference.Detail
	getCatalogErr error
	events        *[]string
}

func (root *preflightModelsRoot) GetCatalogModel(
	context.Context, modelinference.GetModelRequest,
) (modelinference.GetModelResult, error) {
	if root.getCatalogErr != nil {
		return modelinference.GetModelResult{}, root.getCatalogErr
	}
	return modelinference.GetModelResult{Model: root.catalog}, nil
}

func (root *preflightModelsRoot) GetModelReadiness(
	context.Context, modelinference.GetModelReadinessRequest,
) (modelinference.GetModelReadinessResult, error) {
	return modelinference.GetModelReadinessResult{}, modelinference.ErrUnsupportedOperation
}

func (root *preflightModelsRoot) PreflightModelAssets(
	context.Context, modelinference.PrepareModelAssetsRequest,
) (modelinference.PreflightModelAssetsResult, error) {
	if root.events != nil {
		*root.events = append(*root.events, "preflight")
	}
	return modelinference.PreflightModelAssetsResult{}, nil
}

func (root *preflightModelsRoot) InvokeModel(
	context.Context, modelinference.InvokeModelRequest,
) (modelinference.InvokeModelResult, error) {
	if root.events != nil {
		*root.events = append(*root.events, "invoke")
	}
	return modelinference.InvokeModelResult{}, nil
}

func preflightTestScope(t *testing.T) modelinference.RuntimeScopeRef {
	t.Helper()
	scope, err := (modelinference.RuntimeScopeRef{}).Parse("cli-preflight:test-scope")
	if err != nil {
		t.Fatalf("parse preflight scope: %v", err)
	}
	return scope
}

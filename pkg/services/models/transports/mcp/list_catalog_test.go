package modelmcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelmcp "github.com/portpowered/infinite-you/pkg/services/models/transports/mcp"
)

func TestBind_ListCatalogSuccessReturnsCatalogSummariesFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeModelsRoot{
		invoked: &invoked,
		listCatalog: func(_ context.Context, request models.ListModelsRequest) (models.ListModelsResult, error) {
			if request.Scope.String() != testRuntimeScopeRef {
				t.Fatalf("scope = %q, want %q", request.Scope.String(), testRuntimeScopeRef)
			}
			return models.ListModelsResult{
				Models: []models.Summary{{
					Name:   "local-model",
					Status: models.StatusReady,
				}},
			}, nil
		},
	}
	operation := modelmcp.Bind(modelmcp.RootBinding{Models: fake})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolListCatalog,
		json.RawMessage(`{"runtimeScopeRef":"`+testRuntimeScopeRef+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_catalog) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake models root was not invoked")
	}
	var response modelmcp.ToolResponse[models.ListModelsResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if len(response.Result.Models) != 1 || response.Result.Models[0].Name != "local-model" {
		t.Fatalf("models = %#v, want one local-model summary", response.Result.Models)
	}
}

func TestBind_ListCatalogRuntimeScopeErrorsReturnTypedDomainEnvelopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rootErr       error
		wantCode      string
		wantRetryable bool
	}{
		{
			name:          "invalid",
			rootErr:       models.ErrRuntimeScopeInvalid,
			wantCode:      "model.runtime_scope.invalid",
			wantRetryable: false,
		},
		{
			name:          "stale",
			rootErr:       models.ErrRuntimeScopeStale,
			wantCode:      "model.runtime_scope.stale",
			wantRetryable: false,
		},
		{
			name:          "foreign",
			rootErr:       models.ErrRuntimeScopeForeign,
			wantCode:      "model.runtime_scope.foreign",
			wantRetryable: false,
		},
		{
			name:          "unavailable catalog",
			rootErr:       models.ErrUnavailable,
			wantCode:      "model.catalog.unavailable",
			wantRetryable: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake := fakeModelsRoot{
				listCatalog: func(context.Context, models.ListModelsRequest) (models.ListModelsResult, error) {
					return models.ListModelsResult{}, test.rootErr
				},
			}
			operation := modelmcp.Bind(modelmcp.RootBinding{Models: fake})
			raw, err := operation(
				context.Background(),
				modelmcp.ToolListCatalog,
				json.RawMessage(`{"runtimeScopeRef":"`+testRuntimeScopeRef+`"}`),
			)
			if err != nil {
				t.Fatalf("CallTool(list_catalog) transport error = %v, want typed tool response", err)
			}
			envelope := assertTypedToolErrorEnvelope(t, raw, test.wantCode, test.wantRetryable)
			if envelope.Details == nil || envelope.Details["reason"] != test.rootErr.Error() {
				t.Fatalf("error.details = %#v, want reason %q", envelope.Details, test.rootErr.Error())
			}
		})
	}
}

func TestBind_ListCatalogInvalidJSONReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := modelmcp.Bind(modelmcp.RootBinding{
		Models: fakeModelsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolListCatalog,
		json.RawMessage(`{"runtimeScopeRef":`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_catalog) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake models root was invoked for invalid JSON decode")
	}
}

func TestBind_ListCatalogMissingRuntimeScopeRefReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := modelmcp.Bind(modelmcp.RootBinding{
		Models: fakeModelsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolListCatalog,
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("CallTool(list_catalog) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake models root was invoked for missing runtimeScopeRef")
	}
}

func assertBadRequestToolResponse(t *testing.T, raw json.RawMessage) {
	t.Helper()
	assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
}

func assertTypedToolErrorEnvelope(
	t *testing.T,
	raw json.RawMessage,
	wantCode string,
	wantRetryable bool,
) *modelmcp.ToolErrorEnvelope {
	t.Helper()

	var response struct {
		Result *json.RawMessage            `json:"result"`
		Error  *modelmcp.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("tool response result = %s, want error envelope only", raw)
	}
	if response.Error == nil {
		t.Fatalf("tool response = %s, want typed error envelope", raw)
	}
	if response.Error.Code != wantCode {
		t.Fatalf("error.code = %q, want %q; envelope = %#v", response.Error.Code, wantCode, response.Error)
	}
	if response.Error.Retryable != wantRetryable {
		t.Fatalf("error.retryable = %v, want %v; envelope = %#v", response.Error.Retryable, wantRetryable, response.Error)
	}
	if strings.TrimSpace(response.Error.Message) == "" {
		t.Fatalf("error.message is required; envelope = %#v", response.Error)
	}
	return response.Error
}

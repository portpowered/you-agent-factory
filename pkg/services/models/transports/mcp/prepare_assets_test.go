package modelmcp_test

import (
	"context"
	"encoding/json"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelmcp "github.com/portpowered/infinite-you/pkg/services/models/transports/mcp"
)

const testPrepareModelName = "local-model"

func TestBind_PrepareAssetsSuccessReturnsOutcomeFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeModelsRoot{
		invoked: &invoked,
		prepareModelAssets: func(_ context.Context, request models.PrepareModelAssetsRequest) (models.PrepareModelAssetsResult, error) {
			if request.Scope.String() != testRuntimeScopeRef {
				t.Fatalf("scope = %q, want %q", request.Scope.String(), testRuntimeScopeRef)
			}
			if request.Name != testPrepareModelName {
				t.Fatalf("name = %q, want %q", request.Name, testPrepareModelName)
			}
			return models.PrepareModelAssetsResult{
				Asset: models.AssetSnapshot{
					ModelName: testPrepareModelName,
					Readiness: models.AssetReadinessAvailable,
				},
				Outcome: models.AssetPreparationPrepared,
			}, nil
		},
	}
	operation := modelmcp.Bind(modelmcp.RootBinding{Models: fake})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolPrepareAssets,
		prepareAssetsInputJSON(testRuntimeScopeRef, testPrepareModelName),
	)
	if err != nil {
		t.Fatalf("CallTool(prepare_assets) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake models root was not invoked")
	}
	var response modelmcp.ToolResponse[models.PrepareModelAssetsResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if response.Result.Outcome != models.AssetPreparationPrepared {
		t.Fatalf("outcome = %q, want %q", response.Result.Outcome, models.AssetPreparationPrepared)
	}
	if response.Result.Asset.ModelName != testPrepareModelName {
		t.Fatalf("asset.modelName = %q, want %q", response.Result.Asset.ModelName, testPrepareModelName)
	}
	if response.Result.Asset.Readiness != models.AssetReadinessAvailable {
		t.Fatalf("asset.readiness = %q, want %q", response.Result.Asset.Readiness, models.AssetReadinessAvailable)
	}
}

func TestBind_PrepareAssetsSuccessEncodesCallToolResultTransport(t *testing.T) {
	t.Parallel()

	fake := fakeModelsRoot{
		prepareModelAssets: func(_ context.Context, _ models.PrepareModelAssetsRequest) (models.PrepareModelAssetsResult, error) {
			return models.PrepareModelAssetsResult{
				Asset: models.AssetSnapshot{
					ModelName: testPrepareModelName,
					Readiness: models.AssetReadinessAvailable,
				},
				Outcome: models.AssetPreparationAlreadyAvailable,
			}, nil
		},
	}
	operation := modelmcp.Bind(modelmcp.RootBinding{Models: fake})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolPrepareAssets,
		prepareAssetsInputJSON(testRuntimeScopeRef, testPrepareModelName),
	)
	if err != nil {
		t.Fatalf("CallTool(prepare_assets) transport error = %v, want typed tool response", err)
	}

	projected, err := modelmcp.MarshalSuccessCallToolResultJSON(raw)
	if err != nil {
		t.Fatalf("MarshalSuccessCallToolResultJSON() error = %v", err)
	}
	var envelope struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError *bool `json:"isError"`
	}
	if err := json.Unmarshal(projected, &envelope); err != nil {
		t.Fatalf("decode CallToolResult envelope: %v", err)
	}
	if len(envelope.Content) != 1 {
		t.Fatalf("content item count = %d, want 1", len(envelope.Content))
	}
	if envelope.Content[0].Type != "text" {
		t.Fatalf("content[0].type = %q, want text", envelope.Content[0].Type)
	}
	if envelope.Content[0].Text != string(raw) {
		t.Fatalf("content[0].text = %q, want serialized tool response %q", envelope.Content[0].Text, raw)
	}
	if envelope.IsError != nil {
		t.Fatalf("isError = %v, want omitted or false for success transport", *envelope.IsError)
	}
}

func TestBind_PrepareAssetsDomainErrorsReturnTypedEnvelopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rootErr       error
		wantCode      string
		wantRetryable bool
	}{
		{
			name:          "missing source",
			rootErr:       models.ErrAssetSourceMissing,
			wantCode:      "model.asset.source_missing",
			wantRetryable: false,
		},
		{
			name:          "unsupported source",
			rootErr:       models.ErrAssetSourceUnsupported,
			wantCode:      "model.asset.source_unsupported",
			wantRetryable: false,
		},
		{
			name:          "integrity failed",
			rootErr:       models.ErrAssetIntegrityFailed,
			wantCode:      "model.asset.integrity_failed",
			wantRetryable: false,
		},
		{
			name:          "preparation interrupted",
			rootErr:       models.ErrAssetPreparationInterrupted,
			wantCode:      "model.asset.preparation_interrupted",
			wantRetryable: true,
		},
		{
			name:          "foreign scope",
			rootErr:       models.ErrRuntimeScopeForeign,
			wantCode:      "model.runtime_scope.foreign",
			wantRetryable: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake := fakeModelsRoot{
				prepareModelAssets: func(context.Context, models.PrepareModelAssetsRequest) (models.PrepareModelAssetsResult, error) {
					return models.PrepareModelAssetsResult{}, test.rootErr
				},
			}
			operation := modelmcp.Bind(modelmcp.RootBinding{Models: fake})
			raw, err := operation(
				context.Background(),
				modelmcp.ToolPrepareAssets,
				prepareAssetsInputJSON(testRuntimeScopeRef, testPrepareModelName),
			)
			if err != nil {
				t.Fatalf("CallTool(prepare_assets) transport error = %v, want typed tool response", err)
			}
			envelope := assertTypedToolErrorEnvelope(t, raw, test.wantCode, test.wantRetryable)
			if envelope.Details == nil || envelope.Details["reason"] != test.rootErr.Error() {
				t.Fatalf("error.details = %#v, want reason %q", envelope.Details, test.rootErr.Error())
			}
		})
	}
}

func TestBind_PrepareAssetsInvalidJSONReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := modelmcp.Bind(modelmcp.RootBinding{
		Models: fakeModelsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolPrepareAssets,
		json.RawMessage(`{"runtimeScopeRef":"`+testRuntimeScopeRef+`",`),
	)
	if err != nil {
		t.Fatalf("CallTool(prepare_assets) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake models root was invoked for invalid JSON decode")
	}
}

func TestBind_PrepareAssetsMissingModelNameReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := modelmcp.Bind(modelmcp.RootBinding{
		Models: fakeModelsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		modelmcp.ToolPrepareAssets,
		json.RawMessage(`{"runtimeScopeRef":"`+testRuntimeScopeRef+`","name":""}`),
	)
	if err != nil {
		t.Fatalf("CallTool(prepare_assets) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake models root was invoked for missing model name")
	}
}

func prepareAssetsInputJSON(runtimeScopeRef, name string) json.RawMessage {
	payload, err := json.Marshal(map[string]string{
		"runtimeScopeRef": runtimeScopeRef,
		"name":            name,
	})
	if err != nil {
		panic(err)
	}
	return payload
}

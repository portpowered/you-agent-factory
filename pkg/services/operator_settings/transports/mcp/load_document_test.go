package operatorsettingsmcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	mcpoperatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/mcp"
)

func TestBind_LoadDocumentSuccessReturnsDetachedFactsFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	scopeID := "local-00000000-0000-4000-8000-000000000020"
	fake := fakeSettingsRoot{
		invoked: &invoked,
		loadDocument: func(request operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			if request.Path != testConfigPath {
				t.Fatalf("path = %q, want %q", request.Path, testConfigPath)
			}
			if request.RequireExisting {
				t.Fatal("RequireExisting = true, want false")
			}
			return operatorsettings.LoadDocumentResult{
				Document: operatorsettings.Document{BackendScopeID: scopeID},
				Path:     request.Path,
				Found:    true,
			}, nil
		},
	}
	raw := mustCallLoadDocument(t, fake, `{"path":"`+testConfigPath+`","requireExisting":false}`)
	if !invoked {
		t.Fatal("fake settings root was not invoked")
	}
	var response mcpoperatorsettings.ToolResponse[operatorsettings.LoadDocumentResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if response.Result.Path != testConfigPath {
		t.Fatalf("Path = %q, want %q", response.Result.Path, testConfigPath)
	}
	if !response.Result.Found {
		t.Fatal("Found = false, want true")
	}
	if response.Result.Document.BackendScopeID != scopeID {
		t.Fatalf("BackendScopeID = %q, want %q", response.Result.Document.BackendScopeID, scopeID)
	}
}

func TestBind_LoadDocumentMalformedReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeSettingsRoot{
		loadDocument: func(_ operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			return operatorsettings.LoadDocumentResult{}, operatorsettings.ErrDocumentMalformed
		},
	}
	raw := mustCallLoadDocument(t, fake, `{"path":"`+testConfigPath+`"}`)
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"operator_settings.document.malformed",
		false,
		testConfigPath,
	)
	if envelope.Message != "operator document is malformed" {
		t.Fatalf("error.message = %q, want malformed document message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_LoadDocumentNotFoundReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeSettingsRoot{
		loadDocument: func(_ operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			return operatorsettings.LoadDocumentResult{}, operatorsettings.ErrDocumentNotFound
		},
	}
	raw := mustCallLoadDocument(t, fake, `{"path":"`+testConfigPath+`","requireExisting":true}`)
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"operator_settings.document.not_found",
		false,
		testConfigPath,
	)
	if envelope.Message != "operator document not found" {
		t.Fatalf("error.message = %q, want not found document message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_LoadDocumentDocumentFailureKindsReturnTypedErrorEnvelopes(t *testing.T) {
	t.Parallel()

	malformedRaw := mustCallLoadDocument(t, fakeSettingsRoot{
		loadDocument: func(_ operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			return operatorsettings.LoadDocumentResult{}, operatorsettings.DocumentFailure{
				Kind:    operatorsettings.DocumentFailureKindMalformed,
				Message: "invalid json",
				Path:    testConfigPath,
			}
		},
	}, `{"path":"`+testConfigPath+`"}`)
	notFoundRaw := mustCallLoadDocument(t, fakeSettingsRoot{
		loadDocument: func(_ operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			return operatorsettings.LoadDocumentResult{}, operatorsettings.DocumentFailure{
				Kind: operatorsettings.DocumentFailureKindNotFound,
				Path: testConfigPath,
			}
		},
	}, `{"path":"`+testConfigPath+`","requireExisting":true}`)

	assertTypedToolErrorEnvelope(t, malformedRaw, "operator_settings.document.malformed", false, testConfigPath)
	assertTypedToolErrorEnvelope(t, notFoundRaw, "operator_settings.document.not_found", false, testConfigPath)
}

func TestBind_LoadDocumentFailuresHaveDistinctTypedCodes(t *testing.T) {
	t.Parallel()

	malformedRaw := mustCallLoadDocument(t, fakeSettingsRoot{
		loadDocument: func(_ operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			return operatorsettings.LoadDocumentResult{}, operatorsettings.ErrDocumentMalformed
		},
	}, `{"path":"`+testConfigPath+`"}`)
	notFoundRaw := mustCallLoadDocument(t, fakeSettingsRoot{
		loadDocument: func(_ operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			return operatorsettings.LoadDocumentResult{}, operatorsettings.ErrDocumentNotFound
		},
	}, `{"path":"`+testConfigPath+`","requireExisting":true}`)

	malformedEnvelope := assertTypedToolErrorEnvelope(t, malformedRaw, "operator_settings.document.malformed", false, testConfigPath)
	notFoundEnvelope := assertTypedToolErrorEnvelope(t, notFoundRaw, "operator_settings.document.not_found", false, testConfigPath)
	if malformedEnvelope.Code == notFoundEnvelope.Code {
		t.Fatalf("malformed and not found error codes should differ: %#v vs %#v", malformedEnvelope, notFoundEnvelope)
	}
}

func TestBind_LoadDocumentContextCanceledBeforeRootReturnsDocumentedEnvelope(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcpoperatorsettings.Bind(mcpoperatorsettings.RootDependencies{
		Settings: fakeSettingsRoot{invoked: &invoked},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, err := operation(
		ctx,
		mcpoperatorsettings.ToolLoadDocument,
		json.RawMessage(`{"path":"`+testConfigPath+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(load_document) transport error = %v, want typed tool response", err)
	}
	if invoked {
		t.Fatal("fake settings root was invoked for pre-canceled context")
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"operator_settings.request.canceled",
		false,
		"",
	)
	if envelope.Message != "operator settings request was canceled" {
		t.Fatalf("error.message = %q, want canceled request message; envelope = %#v", envelope.Message, envelope)
	}
}

func mustCallLoadDocument(t *testing.T, fake fakeSettingsRoot, inputJSON string) json.RawMessage {
	t.Helper()

	operation := mcpoperatorsettings.Bind(mcpoperatorsettings.RootDependencies{Settings: fake})
	raw, err := operation(
		context.Background(),
		mcpoperatorsettings.ToolLoadDocument,
		json.RawMessage(inputJSON),
	)
	if err != nil {
		t.Fatalf("CallTool(load_document) transport error = %v", err)
	}
	return raw
}

func assertTypedToolErrorEnvelope(
	t *testing.T,
	raw json.RawMessage,
	wantCode string,
	wantRetryable bool,
	wantPath string,
) *mcpoperatorsettings.ToolErrorEnvelope {
	t.Helper()

	var response struct {
		Result *json.RawMessage                       `json:"result"`
		Error  *mcpoperatorsettings.ToolErrorEnvelope `json:"error"`
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
	if wantPath != "" {
		path, ok := response.Error.Details["path"].(string)
		if !ok || path != wantPath {
			t.Fatalf("error.details.path = %#v, want %q; envelope = %#v", response.Error.Details["path"], wantPath, response.Error)
		}
	}
	if strings.TrimSpace(response.Error.Message) == "" {
		t.Fatalf("error.message is required; envelope = %#v", response.Error)
	}
	if response.Error.Details == nil {
		t.Fatalf("error.details is required; envelope = %#v", response.Error)
	}
	if _, ok := response.Error.Details["reason"]; !ok {
		t.Fatalf("error.details.reason is required; envelope = %#v", response.Error)
	}
	return response.Error
}

func TestBind_LoadDocumentInvalidJSONReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcpoperatorsettings.Bind(mcpoperatorsettings.RootDependencies{
		Settings: fakeSettingsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		mcpoperatorsettings.ToolLoadDocument,
		json.RawMessage(`{"path":`),
	)
	if err != nil {
		t.Fatalf("CallTool(load_document) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false, "")
	if !strings.Contains(envelope.Message, "decode load document input") {
		t.Fatalf("error.message = %q, want decode load document input context", envelope.Message)
	}
	if invoked {
		t.Fatal("fake settings root was invoked for invalid JSON decode")
	}
}

func TestLoadDocumentErrorEnvelope_UsesDocumentFailurePathWhenPresent(t *testing.T) {
	t.Parallel()

	failure := operatorsettings.DocumentFailure{
		Kind: operatorsettings.DocumentFailureKindNotFound,
		Path: "/custom/path.json",
	}
	raw := mustCallLoadDocument(t, fakeSettingsRoot{
		loadDocument: func(_ operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			return operatorsettings.LoadDocumentResult{}, failure
		},
	}, `{"path":"`+testConfigPath+`"}`)
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"operator_settings.document.not_found",
		false,
		"/custom/path.json",
	)
	if envelope.Details["reason"] != failure.Error() {
		t.Fatalf("error.details.reason = %#v, want %q", envelope.Details["reason"], failure.Error())
	}
}

func TestBind_LoadDocumentMalformedIncludesRequestPathInDetails(t *testing.T) {
	t.Parallel()

	raw := mustCallLoadDocument(t, fakeSettingsRoot{
		loadDocument: func(_ operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			return operatorsettings.LoadDocumentResult{}, fmt.Errorf("%w", operatorsettings.ErrDocumentMalformed)
		},
	}, `{"path":"`+testConfigPath+`"}`)
	assertTypedToolErrorEnvelope(t, raw, "operator_settings.document.malformed", false, testConfigPath)
}

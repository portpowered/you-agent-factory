package factorydefinition_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionmcp "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestBind_GetCurrentToolEncodesFakeRootResult(t *testing.T) {
	t.Parallel()

	sessionVersion := factorydefinitions.FactoryVersion{
		Logical:  2,
		Physical: time.Unix(0, 2).UTC(),
	}
	factory := mustFactoryFromJSON(t, minimalValidationFactoryBody)
	factory.Name = "beta"
	root := &mcpCapturingCurrentFactoryRootFake{
		getResult: factorydefinitions.EditableFactory{
			Name:     "beta",
			Version:  &sessionVersion,
			Snapshot: mustEditableFactorySnapshot(t, factory),
		},
	}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Definitions: root})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolGetCurrent,
		json.RawMessage(`{"sessionId":"session-2"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_current) error = %v", err)
	}
	if !root.getInvoked {
		t.Fatal("GetCurrentFactoryForSession was not invoked")
	}
	if root.getSessionID != "session-2" {
		t.Fatalf("session id = %q, want session-2", root.getSessionID)
	}

	var response struct {
		Result *factoryapi.Factory                      `json:"result"`
		Error  *factorydefinitionmcp.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("tool response error = %#v, want success result", response.Error)
	}
	if response.Result == nil || response.Result.Name != "beta" {
		t.Fatalf("factory name = %#v, want beta", response.Result)
	}
	if response.Result.Version == nil || response.Result.Version.Logical.Int64() != 2 {
		t.Fatalf("factory version = %#v, want logical 2", response.Result.Version)
	}
}

func TestBind_GetCurrentToolSessionNotFoundReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	root := &mcpCapturingCurrentFactoryRootFake{
		getErr: fmt.Errorf("lookup session: %w", apisurface.ErrFactorySessionNotFound),
	}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Definitions: root})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolGetCurrent,
		json.RawMessage(`{"sessionId":"missing-session"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_current) error = %v", err)
	}

	envelope := assertTypedToolErrorEnvelope(t, raw, "factory_definition.session.not_found", false)
	if envelope.Message != "factory session not found" {
		t.Fatalf("error.message = %q, want factory session not found", envelope.Message)
	}
	if envelope.Details["sessionId"] != "missing-session" {
		t.Fatalf("error.details.sessionId = %#v, want missing-session", envelope.Details["sessionId"])
	}
}

func TestBind_GetCurrentToolCurrentFactoryNotFoundReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	root := &mcpCapturingCurrentFactoryRootFake{
		getErr: factorydefinitions.ErrCurrentFactoryNotFound,
	}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Definitions: root})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolGetCurrent,
		json.RawMessage(`{"sessionId":"session-2"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_current) error = %v", err)
	}

	envelope := assertTypedToolErrorEnvelope(t, raw, "factory_definition.current_factory.not_found", false)
	if envelope.Message != "Current factory not found." {
		t.Fatalf("error.message = %q, want current factory not found message", envelope.Message)
	}
}

func TestBind_SaveCurrentToolDecodesFactoryAndInvokesFakeRoot(t *testing.T) {
	t.Parallel()

	root := &mcpCapturingCurrentFactoryRootFake{
		saveResult: factorydefinitions.EditableFactory{
			Name:     "beta",
			Snapshot: mustEditableFactorySnapshot(t, mustFactoryFromJSON(t, `{"name":"beta","workTypes":[],"workstations":[],"workers":[]}`)),
		},
	}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Definitions: root})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolSaveCurrent,
		json.RawMessage(`{"sessionId":"session-2","factory":`+minimalValidationFactoryBody+`}`),
	)
	if err != nil {
		t.Fatalf("CallTool(save_current) error = %v", err)
	}
	if !root.saveInvoked {
		t.Fatal("Save was not invoked")
	}
	if root.saveSessionID != "session-2" {
		t.Fatalf("save session id = %q, want session-2", root.saveSessionID)
	}
	if root.saveMode != factorydefinitions.SaveModeReplaceCurrent {
		t.Fatalf("save mode = %v, want replace current", root.saveMode)
	}
	if root.saveRequest.Name != "alpha" {
		t.Fatalf("save request name = %q, want alpha", root.saveRequest.Name)
	}

	var response struct {
		Result *factoryapi.Factory                      `json:"result"`
		Error  *factorydefinitionmcp.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("tool response error = %#v, want success result", response.Error)
	}
	if response.Result == nil || response.Result.Name != "beta" {
		t.Fatalf("saved factory name = %#v, want beta", response.Result)
	}
}

func TestBind_SaveCurrentToolStaleVersionReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	root := &mcpCapturingCurrentFactoryRootFake{
		saveErr: factorydefinitions.ErrFactoryVersionStale,
	}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Definitions: root})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolSaveCurrent,
		json.RawMessage(`{"sessionId":"session-2","factory":`+minimalValidationFactoryBody+`}`),
	)
	if err != nil {
		t.Fatalf("CallTool(save_current) error = %v", err)
	}

	envelope := assertTypedToolErrorEnvelope(t, raw, "STALE_FACTORY_VERSION", false)
	if envelope.Message != "Current factory definition is stale. Refresh the graph before saving." {
		t.Fatalf("error.message = %q, want stale version message", envelope.Message)
	}
	targets, ok := envelope.Details["targets"].([]any)
	if !ok || len(targets) != 1 {
		t.Fatalf("error.details.targets = %#v, want stale version target", envelope.Details["targets"])
	}
}

func TestBind_SaveCurrentToolMalformedJSONReturnsBadRequestWithoutInvokingRoot(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "malformed", body: `{"sessionId":"session-2","factory":{"name":"beta"`},
		{name: "empty", body: ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := &mcpCapturingCurrentFactoryRootFake{}
			operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Definitions: root})
			raw, err := operation(
				context.Background(),
				factorydefinitionmcp.ToolSaveCurrent,
				json.RawMessage(tc.body),
			)
			if err != nil {
				t.Fatalf("CallTool(save_current) error = %v", err)
			}
			if root.saveInvoked {
				t.Fatal("Save was invoked before request decode succeeded")
			}
			envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
			if !strings.Contains(envelope.Message, "decode save current input") {
				t.Fatalf("error.message = %q, want decode save current input context", envelope.Message)
			}
		})
	}
}

func TestBind_GetCurrentToolSuccessMatchesCallToolResultTransportPolicy(t *testing.T) {
	t.Parallel()

	root := &mcpCapturingCurrentFactoryRootFake{
		getResult: factorydefinitions.EditableFactory{
			Name:     "beta",
			Snapshot: mustEditableFactorySnapshot(t, mustFactoryFromJSON(t, `{"name":"beta","workTypes":[],"workstations":[],"workers":[]}`)),
		},
	}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Definitions: root})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolGetCurrent,
		json.RawMessage(`{"sessionId":"session-2"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get_current) error = %v", err)
	}

	projected, err := factorydefinitionmcp.MarshalSuccessCallToolResultJSON(raw)
	if err != nil {
		t.Fatalf("MarshalSuccessCallToolResultJSON() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(projected, &decoded); err != nil {
		t.Fatalf("decode CallToolResult: %v", err)
	}
	content, ok := decoded["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("content = %#v, want one text item", decoded["content"])
	}
	item, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("content item = %#v, want object", content[0])
	}
	if item["type"] != "text" {
		t.Fatalf("content type = %#v, want text", item["type"])
	}
	text, ok := item["text"].(string)
	if !ok || !strings.Contains(text, `"name":"beta"`) {
		t.Fatalf("content text = %#v, want serialized success payload", item["text"])
	}
	if decoded["isError"] == true {
		t.Fatalf("CallToolResult isError = true, want false for success transport")
	}
}

type mcpCapturingCurrentFactoryRootFake struct {
	mcpDefinitionsRootFake

	getInvoked   bool
	getSessionID string
	getResult    factorydefinitions.EditableFactory
	getErr       error

	saveInvoked   bool
	saveSessionID string
	saveMode      factorydefinitions.SaveMode
	saveRequest   factorydefinitions.EditableFactory
	saveResult    factorydefinitions.EditableFactory
	saveErr       error
}

func (fake *mcpCapturingCurrentFactoryRootFake) GetCurrentFactoryForSession(
	_ context.Context,
	sessionID string,
) (factorydefinitions.EditableFactory, error) {
	fake.getInvoked = true
	fake.getSessionID = sessionID
	if fake.getErr != nil {
		return factorydefinitions.EditableFactory{}, fake.getErr
	}
	if fake.getResult.Name != "" || fake.getResult.Snapshot != nil {
		return fake.getResult, nil
	}
	return factorydefinitions.EditableFactory{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fake *mcpCapturingCurrentFactoryRootFake) Save(
	_ context.Context,
	sessionID string,
	mode factorydefinitions.SaveMode,
	request factorydefinitions.EditableFactory,
) (factorydefinitions.EditableFactory, error) {
	fake.saveInvoked = true
	fake.saveSessionID = sessionID
	fake.saveMode = mode
	fake.saveRequest = request
	if fake.saveErr != nil {
		return factorydefinitions.EditableFactory{}, fake.saveErr
	}
	if fake.saveResult.Name != "" || fake.saveResult.Snapshot != nil {
		return fake.saveResult, nil
	}
	return request, nil
}

func mustFactoryFromJSON(t *testing.T, body string) factoryapi.Factory {
	t.Helper()

	var factory factoryapi.Factory
	if err := json.Unmarshal([]byte(body), &factory); err != nil {
		t.Fatalf("decode factory JSON: %v", err)
	}
	return factory
}

func mustEditableFactorySnapshot(t *testing.T, factory factoryapi.Factory) *factorydefinitions.FactorySnapshot {
	t.Helper()

	snapshot, err := factorydefinitions.NewFactorySnapshot(factory)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	return snapshot
}

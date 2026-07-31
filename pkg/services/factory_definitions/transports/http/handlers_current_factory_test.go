package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionshttp "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestGetCurrentFactoryBySessionId_EncodesFakeRootResult(t *testing.T) {
	t.Parallel()

	sessionVersion := factorydefinitions.FactoryVersion{
		Logical:  2,
		Physical: time.Unix(0, 2).UTC(),
	}
	factory := mustFactoryFromJSON(t, minimalValidationFactoryBody)
	factory.Name = "beta"
	root := &capturingCurrentFactoryRootFake{
		getResult: factorydefinitions.EditableFactory{
			Name:     "beta",
			Version:  &sessionVersion,
			Snapshot: mustEditableFactorySnapshot(t, factory),
		},
	}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Definitions: root},
		zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/factory-sessions/session-2/factory",
		nil,
	)

	handler.GetCurrentFactoryBySessionId(recorder, request, "session-2")

	if !root.getInvoked {
		t.Fatal("GetCurrentFactoryForSession was not invoked")
	}
	if root.getSessionID != "session-2" {
		t.Fatalf("session id = %q, want session-2", root.getSessionID)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("response = %d %s, want 200", recorder.Code, recorder.Body.String())
	}

	var response factoryapi.Factory
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Name != "beta" {
		t.Fatalf("factory name = %q, want beta", response.Name)
	}
	if response.Version == nil || response.Version.Logical.Int64() != 2 {
		t.Fatalf("factory version = %#v, want logical 2", response.Version)
	}
}

func TestSaveCurrentFactoryBySessionId_RejectsInvalidPayloadBeforeRootInvoked(t *testing.T) {
	t.Parallel()

	cases := []validationDecodeCase{
		{
			name:        "unknown_field",
			body:        `{"factory":{"name":"beta"},"unknownExtra":1}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "malformed",
			body:        `{"factory":{"name":"beta"`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "empty",
			body:        "",
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "multi_object",
			body:        `{"factory":{"name":"beta"}}{}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "request payload must contain one JSON object",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := &capturingCurrentFactoryRootFake{}
			handler := factorydefinitionshttp.NewHandlerFromRoot(
				factorydefinitionshttp.RootBinding{Definitions: root},
				zap.NewNop(),
			)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(
				http.MethodPut,
				"/factory-sessions/session-2/factory",
				strings.NewReader(tc.body),
			)
			request.Header.Set("Content-Type", "application/json")

			handler.SaveCurrentFactoryBySessionId(recorder, request, "session-2")

			if root.saveInvoked {
				t.Fatal("Save was invoked before request decode succeeded")
			}
			assertCurrentFactoryDecodeError(t, recorder, tc.wantStatus, tc.wantCode, tc.wantMessage)
		})
	}
}

func TestSaveCurrentFactoryBySessionId_DecodesFactoryAndInvokesFakeRoot(t *testing.T) {
	t.Parallel()

	root := &capturingCurrentFactoryRootFake{
		saveResult: factorydefinitions.EditableFactory{
			Name:     "beta",
			Snapshot: mustEditableFactorySnapshot(t, mustFactoryFromJSON(t, `{"name":"beta","workTypes":[],"workstations":[],"workers":[]}`)),
		},
	}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Definitions: root},
		zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPut,
		"/factory-sessions/session-2/factory",
		strings.NewReader(saveCurrentFactoryRequestBody(minimalValidationFactoryBody)),
	)
	request.Header.Set("Content-Type", "application/json")

	handler.SaveCurrentFactoryBySessionId(recorder, request, "session-2")

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
	if recorder.Code != http.StatusOK {
		t.Fatalf("response = %d %s, want 200", recorder.Code, recorder.Body.String())
	}

	var response factoryapi.Factory
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Name != "beta" {
		t.Fatalf("saved factory name = %q, want beta", response.Name)
	}
}

type capturingCurrentFactoryRootFake struct {
	httpDefinitionsRootFake

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

func (fake *capturingCurrentFactoryRootFake) GetCurrentFactoryForSession(
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

func (fake *capturingCurrentFactoryRootFake) Save(
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

func saveCurrentFactoryRequestBody(factoryJSON string) string {
	return `{"factory":` + factoryJSON + `}`
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

func assertCurrentFactoryDecodeError(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	wantStatus int,
	wantCode string,
	wantMessage string,
) {
	t.Helper()

	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, wantStatus, recorder.Body.String())
	}

	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if string(errResp.Code) != wantCode {
		t.Fatalf("code = %q, want %q", errResp.Code, wantCode)
	}
	if errResp.Message != wantMessage {
		t.Fatalf("message = %q, want %q", errResp.Message, wantMessage)
	}
	if errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("family = %q, want bad_request", errResp.Family)
	}
	if errResp.Targets == nil || len(*errResp.Targets) != 1 {
		t.Fatalf("targets = %#v, want one form payload validation target", errResp.Targets)
	}
}

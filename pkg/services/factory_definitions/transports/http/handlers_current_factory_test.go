package http_test

import (
	"context"
	"encoding/json"
	"errors"
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

func TestListPackagedFactories_MapsStableCatalogAndArtifacts(t *testing.T) {
	t.Parallel()

	root := &packagedFactoryCatalogRootFake{
		listResult: factorydefinitions.ListBuiltInPackagedFactoriesResult{
			Entries: []factorydefinitions.BuiltInPackagedFactoryEntry{
				{Name: "@you/zeta", Project: "builtin-zeta"},
				{Name: "@you/alpha", Project: "builtin-alpha"},
			},
		},
		definitions: map[string]factorydefinitions.PackagedDefinition{
			"@you/zeta":  packagedFactoryDefinition("@you/zeta", "builtin-zeta", "name: zeta\n"),
			"@you/alpha": packagedFactoryDefinition("@you/alpha", "builtin-alpha", "name: alpha\n"),
		},
	}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Definitions: root},
		zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	handler.ListPackagedFactories(
		recorder,
		httptest.NewRequest(http.MethodGet, "/packaged-factories", nil),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("response = %d %s, want 200", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.PackagedFactoryCatalogResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Factories) != 2 {
		t.Fatalf("factories = %d, want 2", len(response.Factories))
	}
	if response.Factories[0].Name != "@you/alpha" || response.Factories[1].Name != "@you/zeta" {
		t.Fatalf("factory order = [%q, %q], want sorted public names", response.Factories[0].Name, response.Factories[1].Name)
	}
	entry := response.Factories[0]
	if entry.Project != "builtin-alpha" || entry.Slug != "alpha" {
		t.Fatalf("identity = %#v, want alpha/builtin-alpha/alpha", entry)
	}
	if entry.Description.Value != "Description for @you/alpha" {
		t.Fatalf("description = %#v, want stable customer-facing metadata", entry.Description)
	}
	if len(entry.Examples) != 1 || entry.Examples[0].Name != "run-alpha" {
		t.Fatalf("examples = %#v, want one stable invocation example", entry.Examples)
	}
	if entry.Json["name"] != "@you/alpha" {
		t.Fatalf("JSON artifact name = %#v, want @you/alpha", entry.Json["name"])
	}
	if entry.Yaml != "name: alpha\n" {
		t.Fatalf("YAML artifact = %q, want source artifact", entry.Yaml)
	}
}

func TestListPackagedFactories_EmitsEmptyCollection(t *testing.T) {
	t.Parallel()

	root := &packagedFactoryCatalogRootFake{
		listResult: factorydefinitions.ListBuiltInPackagedFactoriesResult{
			Entries: []factorydefinitions.BuiltInPackagedFactoryEntry{},
		},
	}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Definitions: root},
		zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	handler.ListPackagedFactories(
		recorder,
		httptest.NewRequest(http.MethodGet, "/packaged-factories", nil),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("response = %d %s, want 200", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.PackagedFactoryCatalogResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Factories == nil || len(response.Factories) != 0 {
		t.Fatalf("factories = %#v, want a valid empty collection", response.Factories)
	}
}

func TestListPackagedFactories_HidesCatalogLoadAndDecodeFailures(t *testing.T) {
	t.Parallel()

	t.Run("list failure", func(t *testing.T) {
		root := &packagedFactoryCatalogRootFake{
			listErr: errors.New("secret factory payload: do not expose"),
		}
		recorder := httptest.NewRecorder()
		factorydefinitionshttp.NewHandlerFromRoot(
			factorydefinitionshttp.RootBinding{Definitions: root},
			zap.NewNop(),
		).ListPackagedFactories(
			recorder,
			httptest.NewRequest(http.MethodGet, "/packaged-factories", nil),
		)
		assertPackagedFactoryCatalogInternalError(t, recorder, "secret factory payload: do not expose")
	})

	t.Run("artifact decode failure", func(t *testing.T) {
		root := &packagedFactoryCatalogRootFake{
			listResult: factorydefinitions.ListBuiltInPackagedFactoriesResult{
				Entries: []factorydefinitions.BuiltInPackagedFactoryEntry{{
					Name: "@you/broken", Project: "builtin-broken",
				}},
			},
			definitions: map[string]factorydefinitions.PackagedDefinition{
				"@you/broken": {
					Name: "@you/broken", Project: "builtin-broken",
					JSON: []byte(`{"name":`), YAML: []byte("name: broken\n"),
				},
			},
		}
		recorder := httptest.NewRecorder()
		factorydefinitionshttp.NewHandlerFromRoot(
			factorydefinitionshttp.RootBinding{Definitions: root},
			zap.NewNop(),
		).ListPackagedFactories(
			recorder,
			httptest.NewRequest(http.MethodGet, "/packaged-factories", nil),
		)
		assertPackagedFactoryCatalogInternalError(t, recorder, "packaged Factory")
	})
}

func TestListPackagedFactories_RejectsMissingDefinitionsRoot(t *testing.T) {
	t.Parallel()

	recorder := listPackagedFactoriesResponse(t, factorydefinitionshttp.RootBinding{})
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}

func TestListPackagedFactories_RejectsResolveFailure(t *testing.T) {
	t.Parallel()

	root := &packagedFactoryCatalogRootFake{
		listResult: factorydefinitions.ListBuiltInPackagedFactoriesResult{
			Entries: []factorydefinitions.BuiltInPackagedFactoryEntry{{
				Name: "@you/missing", Project: "builtin-missing",
			}},
		},
	}
	recorder := listPackagedFactoriesResponse(t, factorydefinitionshttp.RootBinding{Definitions: root})
	assertPackagedFactoryCatalogInternalError(t, recorder, "missing")
}

func TestListPackagedFactories_RejectsIdentityMismatch(t *testing.T) {
	t.Parallel()

	root := &packagedFactoryCatalogRootFake{
		listResult: factorydefinitions.ListBuiltInPackagedFactoriesResult{
			Entries: []factorydefinitions.BuiltInPackagedFactoryEntry{{
				Name: "@you/mismatch", Project: "listed-project",
			}},
		},
		definitions: map[string]factorydefinitions.PackagedDefinition{
			"@you/mismatch": packagedFactoryDefinition("@you/mismatch", "resolved-project", "name: mismatch\n"),
		},
	}
	recorder := listPackagedFactoriesResponse(t, factorydefinitionshttp.RootBinding{Definitions: root})
	assertPackagedFactoryCatalogInternalError(t, recorder, "identity mismatch")
}

func TestListPackagedFactories_RejectsIncompleteDefinitions(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name       string
		definition factorydefinitions.PackagedDefinition
	}{
		{
			name: "incomplete identity",
			definition: factorydefinitions.PackagedDefinition{
				Name: "", Project: "builtin-incomplete-identity",
			},
		},
		{
			name: "incomplete artifacts",
			definition: factorydefinitions.PackagedDefinition{
				Name: "@you/incomplete-artifacts", Project: "builtin-incomplete-artifacts",
			},
		},
		{
			name: "incomplete discovery metadata",
			definition: factorydefinitions.PackagedDefinition{
				Name: "@you/incomplete-metadata", Project: "builtin-incomplete-metadata",
				JSON: []byte(`{"name":"@you/incomplete-metadata"}`), YAML: []byte("name: incomplete-metadata\n"),
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			root := &packagedFactoryCatalogRootFake{
				listResult: factorydefinitions.ListBuiltInPackagedFactoriesResult{
					Entries: []factorydefinitions.BuiltInPackagedFactoryEntry{{
						Name: testCase.definition.Name, Project: testCase.definition.Project,
					}},
				},
				definitions: map[string]factorydefinitions.PackagedDefinition{
					testCase.definition.Name: testCase.definition,
				},
			}
			recorder := listPackagedFactoriesResponse(t, factorydefinitionshttp.RootBinding{Definitions: root})
			assertPackagedFactoryCatalogInternalError(t, recorder, testCase.name)
		})
	}
}

func TestListPackagedFactories_MapsTypedRootFailure(t *testing.T) {
	t.Parallel()

	root := &packagedFactoryCatalogRootFake{
		listErr: factorydefinitions.ErrInvalidFactoryDefinitionPayload,
	}
	recorder := listPackagedFactoriesResponse(t, factorydefinitionshttp.RootBinding{Definitions: root})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
}

func listPackagedFactoriesResponse(
	t *testing.T,
	binding factorydefinitionshttp.RootBinding,
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	factorydefinitionshttp.NewHandlerFromRoot(binding, zap.NewNop()).ListPackagedFactories(
		recorder,
		httpTestRequest("/packaged-factories"),
	)
	return recorder
}

func httpTestRequest(path string) *http.Request {
	return httptest.NewRequest(http.MethodGet, path, nil)
}

func assertPackagedFactoryCatalogInternalError(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	forbidden string,
) {
	t.Helper()
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), forbidden) {
		t.Fatalf("response exposed sensitive catalog detail %q: %s", forbidden, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
		t.Fatalf("code = %q, want INTERNAL_ERROR", response.Code)
	}
	if response.Message != "failed to load packaged factory catalog" {
		t.Fatalf("message = %q, want actionable catalog failure", response.Message)
	}
}

type packagedFactoryCatalogRootFake struct {
	httpDefinitionsRootFake
	listResult  factorydefinitions.ListBuiltInPackagedFactoriesResult
	listErr     error
	definitions map[string]factorydefinitions.PackagedDefinition
}

func (fake *packagedFactoryCatalogRootFake) ListBuiltInPackagedFactories(
	context.Context,
	factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	return fake.listResult, fake.listErr
}

func (fake *packagedFactoryCatalogRootFake) ResolveBuiltInPackagedFactory(
	_ context.Context,
	request factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
	definition, ok := fake.definitions[request.Name]
	if !ok {
		return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, factorydefinitions.ErrUnknownPackagedFactoryIdentity
	}
	return factorydefinitions.ResolveBuiltInPackagedFactoryResult{
		Definition: definition,
		Formats:    definition.Formats,
	}, nil
}

func packagedFactoryDefinition(
	name string,
	project string,
	yaml string,
) factorydefinitions.PackagedDefinition {
	return factorydefinitions.PackagedDefinition{
		Name:    name,
		Project: project,
		JSON:    []byte(`{"name":"` + name + `","description":{"type":"LOCALIZABLE_ASSET","value":"Description for ` + name + `"},"examples":[{"name":"run-` + strings.TrimPrefix(name, "@you/") + `","description":{"type":"LOCALIZABLE_ASSET","value":"Run ` + name + `"},"args":{"input":"sample"}}]}`),
		YAML:    []byte(yaml),
		Formats: []factorydefinitions.PackagedFactoryFormat{factorydefinitions.PackagedFactoryFormatJSON, factorydefinitions.PackagedFactoryFormatYAML},
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

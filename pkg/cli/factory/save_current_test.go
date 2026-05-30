package factory

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

func TestSaveCurrent_WritesHumanReadableConfirmation(t *testing.T) {
	srv := currentFactorySaveServer(t, factoryapi.Factory{Name: "beta"})
	defer srv.Close()

	var out strings.Builder
	if err := SaveCurrent(SaveCurrentConfig{Server: serverBase(t, srv), Output: &out}); err != nil {
		t.Fatalf("SaveCurrent: %v", err)
	}

	want := "Saved factory beta\nSession: ~default\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestSaveCurrent_JSONEmitsSavedFactoryPayload(t *testing.T) {
	factoryID := "customer-factory"
	srv := currentFactorySaveServer(t, factoryapi.Factory{
		Name: "beta",
		Id:   &factoryID,
	})
	defer srv.Close()

	var out bytes.Buffer
	if err := SaveCurrent(SaveCurrentConfig{Server: serverBase(t, srv), JSON: true, Output: &out}); err != nil {
		t.Fatalf("SaveCurrent: %v", err)
	}
	if bytes.Contains(out.Bytes(), []byte("Saved factory")) {
		t.Fatalf("json output included human-readable text: %q", out.String())
	}

	var got factoryapi.Factory
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Name != "beta" || got.Id == nil || *got.Id != factoryID {
		t.Fatalf("factory JSON = %#v, want beta with id %q", got, factoryID)
	}
}

func TestSaveCurrent_PutSendsCurrentFactoryIncludingVersion(t *testing.T) {
	versionTime := time.Date(2026, 5, 18, 10, 45, 0, 0, time.UTC)
	current := factoryapi.Factory{
		Name:    "beta",
		Version: &factoryapi.HybridLogicalTimestamp{Logical: 44, Physical: versionTime},
	}
	var putPayload factoryapi.SaveFactoryForSessionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/~default/factory" {
			t.Fatalf("path = %q, want /factory-sessions/~default/factory", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(current); err != nil {
				t.Fatalf("encode GET response: %v", err)
			}
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&putPayload); err != nil {
				t.Fatalf("decode PUT body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(putPayload.Factory); err != nil {
				t.Fatalf("encode PUT response: %v", err)
			}
		default:
			t.Fatalf("method = %s", r.Method)
		}
	}))
	defer srv.Close()

	if err := SaveCurrent(SaveCurrentConfig{Server: serverBase(t, srv), Output: ioDiscard(t)}); err != nil {
		t.Fatalf("SaveCurrent: %v", err)
	}
	if putPayload.Mode == nil || *putPayload.Mode != factoryapi.FactorySaveModeReplaceCurrent {
		t.Fatalf("PUT payload mode = %#v, want REPLACE_CURRENT", putPayload.Mode)
	}
	if putPayload.Factory.Name != "beta" {
		t.Fatalf("PUT payload factory name = %q, want beta", putPayload.Factory.Name)
	}
	if putPayload.Factory.Version == nil || putPayload.Factory.Version.Logical.Int64() != 45 {
		t.Fatalf("PUT payload factory version = %#v, want logical 45", putPayload.Factory.Version)
	}
}

func TestSaveCurrent_UsesSessionScopedPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/session-beta/factory" {
			t.Fatalf("path = %q, want /factory-sessions/session-beta/factory", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet, http.MethodPut:
			if err := json.NewEncoder(w).Encode(factoryapi.Factory{Name: "beta"}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		default:
			t.Fatalf("method = %s", r.Method)
		}
	}))
	defer srv.Close()

	var out strings.Builder
	if err := SaveCurrent(SaveCurrentConfig{
		Server: serverBase(t, srv),
		SessionID: "session-beta",
		Output:    &out,
	}); err != nil {
		t.Fatalf("SaveCurrent: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Session: session-beta\n") {
		t.Fatalf("output = %q, want session-beta label", got)
	}
}

func TestSaveCurrent_ReturnsReachableServerError(t *testing.T) {
	var out bytes.Buffer
	err := SaveCurrent(SaveCurrentConfig{Server: "http://127.0.0.1:1", Output: &out})
	if err == nil {
		t.Fatal("expected save against unreachable server to fail")
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should stay empty on failure, got %q", out.String())
	}
	if !strings.Contains(err.Error(), "factory not reachable at http://127.0.0.1:1/factory-sessions/~default/factory") {
		t.Fatalf("error = %q, want reachability context", err.Error())
	}
}

func TestSaveCurrent_ReturnsActionableCurrentFactoryNotFoundError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Code:    factoryapi.NOTFOUND,
			Message: "Current factory not found.",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := SaveCurrent(SaveCurrentConfig{Server: serverBase(t, srv), JSON: true, Output: &out})
	if !errors.Is(err, ErrCurrentFactoryNotFound) {
		t.Fatalf("SaveCurrent error = %v, want ErrCurrentFactoryNotFound", err)
	}
	if out.Len() != 0 {
		t.Fatalf("json mode should not print success output on error: %q", out.String())
	}
}

func TestSaveCurrent_SurfacesPUTValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(factoryapi.Factory{Name: "beta"}); err != nil {
				t.Fatalf("encode GET response: %v", err)
			}
		case http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
				Code:    "INVALID_FACTORY",
				Message: "Factory payload is not a valid Agent Factory definition.",
			}); err != nil {
				t.Fatalf("encode PUT response: %v", err)
			}
		default:
			t.Fatalf("method = %s", r.Method)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := SaveCurrent(SaveCurrentConfig{Server: serverBase(t, srv), Output: &out})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should stay empty on failure, got %q", out.String())
	}
	want := "save current factory failed (400): Factory payload is not a valid Agent Factory definition."
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestSaveCurrent_SurfacesPUTConflictError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(factoryapi.Factory{Name: "beta"}); err != nil {
				t.Fatalf("encode GET response: %v", err)
			}
		case http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
				Code:    "STALE_FACTORY_VERSION",
				Message: "Factory version is stale.",
			}); err != nil {
				t.Fatalf("encode PUT response: %v", err)
			}
		default:
			t.Fatalf("method = %s", r.Method)
		}
	}))
	defer srv.Close()

	err := SaveCurrent(SaveCurrentConfig{Server: serverBase(t, srv), Output: ioDiscard(t)})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	want := "save current factory failed (409): Factory version is stale."
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestSaveCurrent_JSONVerboseKeepsStdoutParseableAndDiagnosticsSeparate(t *testing.T) {
	srv := currentFactorySaveServer(t, factoryapi.Factory{
		Name:             apisurface.DefaultCurrentFactoryName,
		FactoryDirectory: strPtr(t.TempDir()),
	})
	defer srv.Close()

	var out bytes.Buffer
	var diagnostics bytes.Buffer
	if err := SaveCurrent(SaveCurrentConfig{
		Server: serverBase(t, srv),
		JSON:        true,
		Verbose:     true,
		Output:      &out,
		Diagnostics: &diagnostics,
	}); err != nil {
		t.Fatalf("SaveCurrent: %v", err)
	}
	var got factoryapi.Factory
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json output is not valid Factory JSON: %v\n%s", err, out.String())
	}
	diag := diagnostics.String()
	for _, want := range []string{
		"factory query request",
		"factory save request",
		"factory save response",
		"endpointPath=/factory-sessions/~default/factory",
		"session=~default",
		"status=200",
	} {
		if !strings.Contains(diag, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, diag)
		}
	}
}

func currentFactorySaveServer(t *testing.T, current factoryapi.Factory) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/~default/factory" {
			t.Fatalf("path = %q, want /factory-sessions/~default/factory", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet, http.MethodPut:
			if err := json.NewEncoder(w).Encode(current); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		default:
			t.Fatalf("method = %s", r.Method)
		}
	}))
}

func strPtr(value string) *string {
	return &value
}

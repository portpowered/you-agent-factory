package factory

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCreateFromFile_WritesHumanReadableConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	var out strings.Builder
	if err := createFromFileWithScriptedPersistence(t, CreateFromFileConfig{
		Name:   "gamma",
		From:   from,
		Dir:    rootDir,
		Output: &out,
	}, factorydefinitions.NamedFactoryPersistenceResult{Name: "gamma", FactoryDir: filepath.Join(rootDir, "gamma")}, nil, nil); err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}

	wantDir := filepath.Join(rootDir, "gamma")
	want := "Created factory gamma\nDirectory: " + wantDir + "\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestCreateFromFile_SetCurrentUpdatesPointer(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	if err := createFromFileWithScriptedPersistence(t, CreateFromFileConfig{
		Name:       "gamma",
		From:       from,
		Dir:        rootDir,
		SetCurrent: true,
		Output:     ioDiscard(t),
	}, factorydefinitions.NamedFactoryPersistenceResult{Name: "gamma", FactoryDir: filepath.Join(rootDir, "gamma")}, nil, func(request factorydefinitions.NamedFactoryPersistenceRequest) {
		if !request.SetCurrent {
			t.Fatal("SetCurrent = false, want true")
		}
	}); err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}
}

func TestCreateFromFile_JSONEmitsStructuredConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	var out bytes.Buffer
	if err := createFromFileWithScriptedPersistence(t, CreateFromFileConfig{
		Name:   "gamma",
		From:   from,
		Dir:    rootDir,
		JSON:   true,
		Output: &out,
	}, factorydefinitions.NamedFactoryPersistenceResult{Name: "gamma", FactoryDir: filepath.Join(rootDir, "gamma")}, nil, nil); err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}

	var result CreateFromFileResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if result.Name != "gamma" || result.FactoryDir != filepath.Join(rootDir, "gamma") {
		t.Fatalf("result = %#v, want gamma factory directory", result)
	}
}

func TestReplaceCurrent_WritesHumanReadableConfirmation(t *testing.T) {
	srv := currentFactorySaveServer(t, factoryapi.Factory{Name: "beta"})
	defer srv.Close()

	var out strings.Builder
	if err := NewReplaceCurrent(testHTTPProtocol(t))(ReplaceCurrentConfig{Context: context.Background(), Server: serverBase(t, srv), Output: &out}); err != nil {
		t.Fatalf("ReplaceCurrent: %v", err)
	}

	want := "Replaced current factory beta\nSession: ~default\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestReplaceCurrent_UsesSessionScopedPath(t *testing.T) {
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
	if err := NewReplaceCurrent(testHTTPProtocol(t))(ReplaceCurrentConfig{Context: context.Background(),
		Server:    serverBase(t, srv),
		SessionID: "session-beta",
		Output:    &out,
	}); err != nil {
		t.Fatalf("ReplaceCurrent: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Session: session-beta\n") {
		t.Fatalf("output = %q, want session-beta label", got)
	}
}

func TestReplaceCurrent_SurfacesUnexpectedHTTPErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(factoryapi.Factory{Name: "beta"}); err != nil {
				t.Fatalf("encode GET response: %v", err)
			}
		case http.MethodPut:
			w.WriteHeader(http.StatusBadGateway)
			if _, err := w.Write([]byte("upstream unavailable")); err != nil {
				t.Fatalf("write PUT response: %v", err)
			}
		default:
			t.Fatalf("method = %s", r.Method)
		}
	}))
	defer srv.Close()

	err := NewReplaceCurrent(testHTTPProtocol(t))(ReplaceCurrentConfig{Context: context.Background(), Server: serverBase(t, srv), Output: io.Discard})
	if err == nil {
		t.Fatal("expected error")
	}
	want := "replace current factory failed (502): upstream unavailable"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

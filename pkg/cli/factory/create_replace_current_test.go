package factory

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

func TestCreateFromFile_WritesHumanReadableConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	var out strings.Builder
	if err := CreateFromFile(CreateFromFileConfig{
		Name:   "gamma",
		From:   from,
		Dir:    rootDir,
		Output: &out,
	}); err != nil {
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

	if err := CreateFromFile(CreateFromFileConfig{
		Name:       "gamma",
		From:       from,
		Dir:        rootDir,
		SetCurrent: true,
		Output:     ioDiscard(t),
	}); err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}

	current, err := factoryconfig.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if current != "gamma" {
		t.Fatalf("current = %q, want gamma", current)
	}
}

func TestCreateFromFile_JSONEmitsStructuredConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	var out bytes.Buffer
	if err := CreateFromFile(CreateFromFileConfig{
		Name:   "gamma",
		From:   from,
		Dir:    rootDir,
		JSON:   true,
		Output: &out,
	}); err != nil {
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
	if err := ReplaceCurrent(ReplaceCurrentConfig{Server: serverBase(t, srv), Output: &out}); err != nil {
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
	if err := ReplaceCurrent(ReplaceCurrentConfig{
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

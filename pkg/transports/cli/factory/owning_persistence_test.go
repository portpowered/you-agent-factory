package factory

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestOwningPersistence_CreateNamedFactoryIsDurableWithoutSave(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	if err := CreateFromFile(CreateFromFileConfig{
		Name:   "gamma",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}); err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}

	configPath := filepath.Join(rootDir, "gamma", interfaces.FactoryConfigFile)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}
	if !strings.Contains(string(data), "execute-gamma") {
		t.Fatalf("factory config = %q, want durable gamma workstation body", string(data))
	}

	var out strings.Builder
	if err := List(ListConfig{Dir: rootDir, Output: &out}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(out.String(), "gamma\t"+filepath.Join(rootDir, "gamma")) {
		t.Fatalf("list output = %q, want durable gamma factory row", out.String())
	}
}

func TestOwningPersistence_CreateNamedFactoryRejectsDuplicateName(t *testing.T) {
	rootDir := t.TempDir()
	payload := saveTestNamedFactoryPayload(t, "alpha")
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", payload); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	from := writeFactoryConfigFile(t, rootDir, "alpha-copy", payload)

	err := CreateFromFile(CreateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	})
	if err == nil {
		t.Fatal("expected duplicate factory name to fail")
	}
	if !strings.Contains(err.Error(), "factory already exists") {
		t.Fatalf("error = %v, want already-exists message", err)
	}
}

func TestOwningPersistence_UpdateNamedFactoryReplacesWithoutSave(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	from := writeFactoryConfigFile(t, rootDir, "alpha-updated", saveTestNamedFactoryPayload(t, "alpha-v2"))

	if err := UpdateFromFile(UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}); err != nil {
		t.Fatalf("UpdateFromFile: %v", err)
	}

	configPath := filepath.Join(rootDir, "alpha", interfaces.FactoryConfigFile)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}
	if !strings.Contains(string(data), "execute-alpha-v2") {
		t.Fatalf("factory config = %q, want updated workstation body", string(data))
	}
}

func TestOwningPersistence_UpdateNamedFactoryRejectsMissingName(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha"))

	err := UpdateFromFile(UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	})
	if err == nil {
		t.Fatal("expected missing factory name to fail")
	}
	if !strings.Contains(err.Error(), "factory not found") {
		t.Fatalf("error = %v, want not-found message", err)
	}
}

func TestOwningPersistence_InvalidUpdateDoesNotCorruptExistingFactory(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	from := writeFactoryConfigFile(t, rootDir, "invalid", []byte(factoryfixtures.CrossPathInvalidFactoryJSON))

	err := UpdateFromFile(UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	})
	if err == nil {
		t.Fatal("expected invalid factory topology to fail")
	}

	configPath := filepath.Join(rootDir, "alpha", interfaces.FactoryConfigFile)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}
	if !strings.Contains(string(data), "execute-alpha") {
		t.Fatalf("factory config = %q, want original alpha workstation body preserved", string(data))
	}
}

func TestOwningPersistence_ReplaceCurrentPersistsWithoutSave(t *testing.T) {
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

	if err := ReplaceCurrent(ReplaceCurrentConfig{
		Server: serverBase(t, srv),
		Output: ioDiscard(t),
	}); err != nil {
		t.Fatalf("ReplaceCurrent: %v", err)
	}
	if putPayload.Mode == nil || *putPayload.Mode != factoryapi.FactorySaveModeReplaceCurrent {
		t.Fatalf("PUT payload mode = %#v, want REPLACE_CURRENT", putPayload.Mode)
	}
	if putPayload.Factory.Version == nil || putPayload.Factory.Version.Logical.Int64() != 45 {
		t.Fatalf("PUT payload factory version = %#v, want logical 45", putPayload.Factory.Version)
	}
}

func TestOwningPersistence_CreateNamedFactoryLeavesUnrelatedFactoriesIntact(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	if err := CreateFromFile(CreateFromFileConfig{
		Name:   "gamma",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}); err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}

	alphaPath := filepath.Join(rootDir, "alpha", interfaces.FactoryConfigFile)
	data, err := os.ReadFile(alphaPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", alphaPath, err)
	}
	if !strings.Contains(string(data), "execute-alpha") {
		t.Fatalf("alpha factory config = %q, want original alpha workstation body preserved", string(data))
	}
}

func TestOwningPersistence_ReplaceCurrentRejectsStaleVersionWithoutWriting(t *testing.T) {
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

	err := ReplaceCurrent(ReplaceCurrentConfig{Server: serverBase(t, srv), Output: ioDiscard(t)})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	want := "replace current factory failed (409): Factory version is stale."
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

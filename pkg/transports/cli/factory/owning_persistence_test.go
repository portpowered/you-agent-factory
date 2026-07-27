package factory

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestOwningPersistence_CreateNamedFactoryIsDurableWithoutSave(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	if err := createFromFileWithScriptedPersistence(t, CreateFromFileConfig{
		Name:   "gamma",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}, interfaces.NamedFactoryPersistenceResult{Name: "gamma", FactoryDir: filepath.Join(rootDir, "gamma")}, nil, func(request interfaces.NamedFactoryPersistenceRequest) {
		if request.Mode != interfaces.NamedFactoryPersistenceModeCreate || request.Name != "gamma" || !strings.Contains(string(request.Payload), "execute-gamma") {
			t.Fatalf("request = %#v, want durable create operation", request)
		}
	}); err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}

	useNamedFactoryCatalogFake(t, namedFactoryCatalogFake{
		list: func(string) ([]interfaces.NamedFactoryListEntry, error) {
			return []interfaces.NamedFactoryListEntry{{
				Name:       "gamma",
				FactoryDir: filepath.Join(rootDir, "gamma"),
			}}, nil
		},
	})
	var out strings.Builder
	if err := testList(ListConfig{ProjectRoot: rootDir, GlobalRoot: rootDir, Output: &out}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(out.String(), "gamma\t"+filepath.Join(rootDir, "gamma")) {
		t.Fatalf("list output = %q, want durable gamma factory row", out.String())
	}
}

func TestOwningPersistence_CreateNamedFactoryRejectsDuplicateName(t *testing.T) {
	rootDir := t.TempDir()
	payload := saveTestNamedFactoryPayload(t, "alpha")
	from := writeFactoryConfigFile(t, rootDir, "alpha-copy", payload)

	err := createFromFileWithScriptedPersistence(t, CreateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}, interfaces.NamedFactoryPersistenceResult{}, interfaces.ErrNamedFactoryAlreadyExists, nil)

	if err == nil {
		t.Fatal("expected duplicate factory name to fail")
	}
	if !strings.Contains(err.Error(), "factory already exists") {
		t.Fatalf("error = %v, want already-exists message", err)
	}
}

func TestOwningPersistence_UpdateNamedFactoryReplacesWithoutSave(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "alpha-updated", saveTestNamedFactoryPayload(t, "alpha-v2"))

	if err := updateFromFileWithScriptedPersistence(t, UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}, interfaces.NamedFactoryPersistenceResult{Name: "alpha", FactoryDir: filepath.Join(rootDir, "alpha")}, nil, func(request interfaces.NamedFactoryPersistenceRequest) {
		if request.Mode != interfaces.NamedFactoryPersistenceModeReplace || !strings.Contains(string(request.Payload), "execute-alpha-v2") {
			t.Fatalf("request = %#v, want replacement payload", request)
		}
	}); err != nil {
		t.Fatalf("UpdateFromFile: %v", err)
	}
}

func TestOwningPersistence_UpdateNamedFactoryRejectsMissingName(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha"))

	err := updateFromFileWithScriptedPersistence(t, UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}, interfaces.NamedFactoryPersistenceResult{}, fs.ErrNotExist, nil)

	if err == nil {
		t.Fatal("expected missing factory name to fail")
	}
	if !strings.Contains(err.Error(), "factory not found") {
		t.Fatalf("error = %v, want not-found message", err)
	}
}

func TestOwningPersistence_InvalidUpdateDoesNotCorruptExistingFactory(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "invalid", saveTestNamedFactoryPayload(t, "invalid"))

	err := updateFromFileWithScriptedPersistence(t, UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}, interfaces.NamedFactoryPersistenceResult{}, &interfaces.BlockingFactoryLoadError{Targets: []interfaces.ValidationTarget{{
		Code:    interfaces.ValidationCodeFactoryPayloadInvalid,
		Message: "Factory topology contains invalid graph references.",
	}}}, func(request interfaces.NamedFactoryPersistenceRequest) {
		if request.Mode != interfaces.NamedFactoryPersistenceModeReplace {
			t.Fatalf("mode = %q, want replace", request.Mode)
		}
	})

	if err == nil {
		t.Fatal("expected invalid factory topology to fail")
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

	if err := NewReplaceCurrent(testHTTPProtocol(t))(ReplaceCurrentConfig{Context: context.Background(),
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
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	if err := createFromFileWithScriptedPersistence(t, CreateFromFileConfig{
		Name:   "gamma",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}, interfaces.NamedFactoryPersistenceResult{Name: "gamma", FactoryDir: filepath.Join(rootDir, "gamma")}, nil, func(request interfaces.NamedFactoryPersistenceRequest) {
		if request.RootDir != rootDir || request.Name != "gamma" {
			t.Fatalf("request = %#v, want isolated gamma create", request)
		}
	}); err != nil {
		t.Fatalf("CreateFromFile: %v", err)
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

	err := NewReplaceCurrent(testHTTPProtocol(t))(ReplaceCurrentConfig{Context: context.Background(), Server: serverBase(t, srv), Output: ioDiscard(t)})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	want := "replace current factory failed (409): Factory version is stale."
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

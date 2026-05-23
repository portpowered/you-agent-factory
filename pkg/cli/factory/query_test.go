package factory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

func TestQuery_WritesHumanReadableDefaultRootFactory(t *testing.T) {
	factoryDir := t.TempDir()
	srv := currentFactoryServer(t, factoryapi.Factory{
		Name:             apisurface.DefaultCurrentFactoryName,
		FactoryDirectory: &factoryDir,
	})
	defer srv.Close()

	var out strings.Builder
	if err := Query(QueryConfig{Port: serverPort(t, srv), Output: &out}); err != nil {
		t.Fatalf("Query: %v", err)
	}

	want := "NAME\tKIND\tID\tFACTORY DIRECTORY\n" +
		fmt.Sprintf("%s\tdefault-root\t\t%s\n", apisurface.DefaultCurrentFactoryName, factoryDir)
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestQuery_WritesHumanReadableNamedFactory(t *testing.T) {
	factoryID := "customer-factory"
	srv := currentFactoryServer(t, factoryapi.Factory{
		Name: "beta",
		Id:   &factoryID,
	})
	defer srv.Close()

	var out strings.Builder
	if err := Query(QueryConfig{Port: serverPort(t, srv), Output: &out}); err != nil {
		t.Fatalf("Query: %v", err)
	}

	want := "NAME\tKIND\tID\tFACTORY DIRECTORY\n" +
		"beta\tnamed\tcustomer-factory\t\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestQuery_WritesJSONDefaultRootFactory(t *testing.T) {
	factoryDir := t.TempDir()
	srv := currentFactoryServer(t, factoryapi.Factory{
		Name:             apisurface.DefaultCurrentFactoryName,
		FactoryDirectory: &factoryDir,
	})
	defer srv.Close()

	var out bytes.Buffer
	if err := Query(QueryConfig{Port: serverPort(t, srv), JSON: true, Output: &out}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if bytes.Contains(out.Bytes(), []byte("NAME\tKIND")) {
		t.Fatalf("json output included human-readable text: %q", out.String())
	}

	var got factoryapi.Factory
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json output is not valid Factory JSON: %v\n%s", err, out.String())
	}
	if got.Name != apisurface.DefaultCurrentFactoryName {
		t.Fatalf("current factory name = %q, want %q", got.Name, apisurface.DefaultCurrentFactoryName)
	}
	if got.FactoryDirectory == nil || *got.FactoryDirectory != factoryDir {
		t.Fatalf("factory directory = %#v, want %q", got.FactoryDirectory, factoryDir)
	}
}

func TestQuery_WritesJSONNamedFactory(t *testing.T) {
	factoryID := "customer-factory"
	workers := []factoryapi.Worker{{Name: "executor"}}
	srv := currentFactoryServer(t, factoryapi.Factory{
		Name:    "beta",
		Id:      &factoryID,
		Workers: &workers,
	})
	defer srv.Close()

	var out bytes.Buffer
	if err := Query(QueryConfig{Port: serverPort(t, srv), JSON: true, Output: &out}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if bytes.Contains(out.Bytes(), []byte("NAME\tKIND")) {
		t.Fatalf("json output included human-readable text: %q", out.String())
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json output is not valid Factory JSON: %v\n%s", err, out.String())
	}
	if got["name"] != "beta" || got["id"] != factoryID {
		t.Fatalf("factory JSON = %#v, want name beta and id %q", got, factoryID)
	}
	workerPayloads, ok := got["workers"].([]any)
	if !ok || len(workerPayloads) != 1 {
		t.Fatalf("workers = %#v, want one API worker payload", got["workers"])
	}
}

func TestQuery_ReturnsActionableCurrentFactoryNotFoundError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory/~current" {
			t.Fatalf("path = %q, want /factory/~current", r.URL.Path)
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
	err := Query(QueryConfig{Port: serverPort(t, srv), JSON: true, Output: &out})
	if !errors.Is(err, ErrCurrentFactoryNotFound) {
		t.Fatalf("Query error = %v, want ErrCurrentFactoryNotFound", err)
	}
	if out.Len() != 0 {
		t.Fatalf("json mode should not print success output on error: %q", out.String())
	}
	want := "running service has no active current factory; start a factory or activate a named factory: current factory not found: Current factory not found."
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestQuery_ReturnsReachableServerError(t *testing.T) {
	var out bytes.Buffer
	err := Query(QueryConfig{Port: 1, Output: &out})
	if err == nil {
		t.Fatal("expected query against unreachable server to fail")
	}
	if out.Len() != 0 {
		t.Fatalf("human mode should not print success output on error: %q", out.String())
	}
	if !strings.Contains(err.Error(), "factory not reachable at http://localhost:1/factory/~current") {
		t.Fatalf("error = %q, want reachability context", err.Error())
	}
}

func TestQueryCurrent_ReturnsDefaultRootFactory(t *testing.T) {
	factoryDir := t.TempDir()
	srv := currentFactoryServer(t, factoryapi.Factory{
		Name:             apisurface.DefaultCurrentFactoryName,
		FactoryDirectory: &factoryDir,
	})
	defer srv.Close()

	current, err := QueryCurrent(QueryCurrentConfig{Port: serverPort(t, srv)})
	if err != nil {
		t.Fatalf("QueryCurrent: %v", err)
	}

	if current.Name != apisurface.DefaultCurrentFactoryName {
		t.Fatalf("current factory name = %q, want %q", current.Name, apisurface.DefaultCurrentFactoryName)
	}
	if current.FactoryDirectory == nil || *current.FactoryDirectory != factoryDir {
		t.Fatalf("factory directory = %#v, want %q", current.FactoryDirectory, factoryDir)
	}
}

func TestQueryCurrent_ReturnsNamedFactory(t *testing.T) {
	factoryID := "customer-factory"
	srv := currentFactoryServer(t, factoryapi.Factory{
		Name: "beta",
		Id:   &factoryID,
	})
	defer srv.Close()

	current, err := QueryCurrent(QueryCurrentConfig{Port: serverPort(t, srv)})
	if err != nil {
		t.Fatalf("QueryCurrent: %v", err)
	}

	if current.Name != "beta" {
		t.Fatalf("current factory name = %q, want beta", current.Name)
	}
	if current.Id == nil || *current.Id != factoryID {
		t.Fatalf("factory id = %#v, want %q", current.Id, factoryID)
	}
}

func TestQueryCurrent_ReturnsInspectableNotFoundError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory/~current" {
			t.Fatalf("path = %q, want /factory/~current", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Code:    "NOT_FOUND",
			Message: "Current factory not found.",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	_, err := QueryCurrent(QueryCurrentConfig{Port: serverPort(t, srv)})
	if !errors.Is(err, ErrCurrentFactoryNotFound) {
		t.Fatalf("QueryCurrent error = %v, want ErrCurrentFactoryNotFound", err)
	}
	if got := err.Error(); got != "current factory not found: Current factory not found." {
		t.Fatalf("error = %q", got)
	}
}

func TestQueryCurrent_PreservesUnexpectedResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory/~current" {
			t.Fatalf("path = %q, want /factory/~current", r.URL.Path)
		}
		http.Error(w, "backend exploded", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := QueryCurrent(QueryCurrentConfig{Port: serverPort(t, srv)})
	if err == nil {
		t.Fatal("expected unexpected server error")
	}
	if got := err.Error(); got != "query current factory failed (500): backend exploded" {
		t.Fatalf("error = %q", got)
	}
}

func currentFactoryServer(t *testing.T, current factoryapi.Factory) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory/~current" {
			t.Fatalf("path = %q, want /factory/~current", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(current); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
}

func serverPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()

	var port int
	if _, err := fmt.Sscanf(srv.URL, "http://127.0.0.1:%d", &port); err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return port
}

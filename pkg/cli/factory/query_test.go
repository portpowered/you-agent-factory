package factory

import (
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
			Message: "Current named factory not found.",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	_, err := QueryCurrent(QueryCurrentConfig{Port: serverPort(t, srv)})
	if !errors.Is(err, ErrCurrentFactoryNotFound) {
		t.Fatalf("QueryCurrent error = %v, want ErrCurrentFactoryNotFound", err)
	}
	if got := err.Error(); got != "current factory not found: Current named factory not found." {
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

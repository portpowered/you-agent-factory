package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factorycli "github.com/portpowered/infinite-you/pkg/cli/factory"
)

func TestFactoryQueryCommand_ServerFlagReachesHTTPTestServer(t *testing.T) {
	factoryDir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/~default/factory" {
			t.Fatalf("path = %q, want /factory-sessions/~default/factory", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.Factory{
			Name:             apisurface.DefaultCurrentFactoryName,
			FactoryDirectory: &factoryDir,
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	originalQueryFactory := queryFactory
	defer func() {
		queryFactory = originalQueryFactory
	}()

	var got factorycli.QueryConfig
	queryFactory = func(cfg factorycli.QueryConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "query", "--server", strings.TrimSuffix(srv.URL, "/")})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory query --server: %v", err)
	}
	if got.Server != strings.TrimSuffix(srv.URL, "/") {
		t.Fatalf("server = %q, want %q", got.Server, strings.TrimSuffix(srv.URL, "/"))
	}
}

func TestFactoryQueryCommand_PortFlagRejected(t *testing.T) {
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "query", "--port", "9090"})

	if execErr := root.Execute(); execErr == nil {
		t.Fatal("expected --port rejection")
	} else if !strings.Contains(execErr.Error(), "--server") {
		t.Fatalf("error = %v, want --server guidance", execErr)
	}
}

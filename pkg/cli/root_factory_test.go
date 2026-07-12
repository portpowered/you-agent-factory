package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/cli/commandidentity"
	factorycli "github.com/portpowered/infinite-you/pkg/cli/factory"
)

var removedFactoryConfigCommandPaths = []string{
	"you config",
	"you config flatten",
	"you config expand",
	"you factory validate",
}

func TestFactoryConfigCommand_OldPathsNotRegistered(t *testing.T) {
	root := NewRootCommand()
	for _, path := range [][]string{
		{"config"},
		{"config", "flatten"},
		{"config", "expand"},
	} {
		if _, _, err := root.Find(path); err == nil {
			t.Fatalf("find %v: expected lookup failure for removed path", path)
		}
	}

	factoryCmd, _, err := root.Find([]string{"factory"})
	if err != nil {
		t.Fatalf("find factory: %v", err)
	}
	for _, child := range factoryCmd.Commands() {
		if child.Name() == "validate" {
			t.Fatalf("factory must not register validate as a direct child")
		}
	}
}

func TestFactoryConfigCommand_OldPathsRejectAtRuntime(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "top-level config", args: []string{"config", "--help"}},
		{name: "config flatten", args: []string{"config", "flatten", "./factory"}},
		{name: "config expand", args: []string{"config", "expand", "./factory.json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootCommand()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(tc.args)
			if err := root.Execute(); err == nil {
				t.Fatal("expected removed command path to fail")
			}
		})
	}
}

func TestFactoryConfigCommand_DirectFactoryValidateDoesNotRun(t *testing.T) {
	originalValidateFactory := validateFactory
	defer func() {
		validateFactory = originalValidateFactory
	}()

	called := false
	validateFactory = func(factorycli.ValidateConfig) error {
		called = true
		return nil
	}

	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "validate", "./factory.json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory validate: %v", err)
	}
	if called {
		t.Fatal("direct you factory validate must not invoke factory validation")
	}
	if !strings.Contains(out.String(), "you factory config validate") {
		t.Fatalf("factory validate should fall back to factory help, got:\n%s", out.String())
	}
}

func TestFactoryConfigCommand_NoHiddenOrDeprecatedWrappers(t *testing.T) {
	root := NewRootCommand()
	inventory, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("walk command tree: %v", err)
	}

	removed := make(map[string]struct{}, len(removedFactoryConfigCommandPaths))
	for _, path := range removedFactoryConfigCommandPaths {
		removed[path] = struct{}{}
	}

	for _, record := range inventory.Commands {
		if _, stillRegistered := removed[record.Path]; stillRegistered {
			t.Fatalf("removed path %q is still registered", record.Path)
		}
		if record.Visibility == "hidden" || record.Lifecycle == "deprecated" {
			for path := range removed {
				if record.Path == path {
					t.Fatalf("%s command %q reintroduces removed path", record.Visibility, record.Path)
				}
			}
		}
	}
}

func TestFactoryCommand_RegistersSubcommands(t *testing.T) {
	root := NewRootCommand()
	for _, path := range [][]string{
		{"factory", "query"},
		{"factory", "list"},
		{"factory", "config"},
		{"factory", "config", "validate"},
		{"factory", "config", "flatten"},
		{"factory", "config", "expand"},
		{"factory", "save"},
		{"factory", "update"},
		{"factory", "delete"},
	} {
		if _, _, err := root.Find(path); err != nil {
			t.Fatalf("find %v: %v", path, err)
		}
	}
}

func TestFactoryCommand_HelpDocumentsSubcommandsAndExamples(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"query",
		"list",
		"config",
		"save",
		"update",
		"delete",
		"global --server",
		"you factory query",
		"you factory config validate",
		"you factory list",
		"you factory save staging --from ./factory.json",
		"you factory update staging --from ./factory.json",
		"you factory delete staging",
		"you factory save",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("factory help missing %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "--port") {
		t.Fatalf("factory help must not advertise --port:\n%s", help)
	}
	if strings.Contains(help, "you factory validate") {
		t.Fatalf("factory help must not advertise direct you factory validate:\n%s", help)
	}
}

func TestFactoryConfigCommand_HelpDocumentsSubcommandsAndExamples(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "config", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory config --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"validate",
		"flatten",
		"expand",
		"you factory config validate ./factory.json",
		"you factory config flatten ./factory",
		"you factory config expand ./factory.json",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("factory config help missing %q:\n%s", want, help)
		}
	}
}

func TestFactoryListCommand_HelpDocumentsProjectAndGlobalRoots(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "list", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory list --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"project-local named factories from ./factory",
		"~/.you-agent-factory/you-agent-factories",
		"never merges project-local and global entries",
		"you factory list --dir ~/.you-agent-factory/you-agent-factories",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("factory list help missing %q:\n%s", want, help)
		}
	}
}

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

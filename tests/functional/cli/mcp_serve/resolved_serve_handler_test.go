package mcp_serve_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	mcpcli "github.com/portpowered/infinite-you/pkg/transports/cli/mcp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/spf13/cobra"
)

func TestResolvedServeHandler_ReportsMissingCanonicalInputsBeforeInitializer(t *testing.T) {
	handler := mcpcli.ResolvedServeHandler(mcpcli.ServeBinding{
		InitializeStdio: func(context.Context, startupcli.MCPIntent) error {
			t.Fatal("initializer must not run before canonical inputs resolve")
			return nil
		},
	})
	cases := []struct {
		name    string
		inputs  resolvedinput.Inputs
		wantErr string
	}{
		{name: "fixture catalog", inputs: resolvedinput.Inputs{}, wantErr: "read MCP fixture catalog input"},
		{name: "runtime", inputs: partialServeInputs(t, true, false, false), wantErr: "read MCP runtime input"},
		{name: "project root", inputs: partialServeInputs(t, true, true, false), wantErr: "read MCP project root input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := handler(&cobra.Command{Use: "serve"}, tc.inputs, resolvedinput.Inputs{})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("handler error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestResolvedServeHandler_RequiresInjectedDependencies(t *testing.T) {
	t.Run("stdio initializer", func(t *testing.T) {
		err := mcpcli.ResolvedServeHandler(mcpcli.ServeBinding{})(
			&cobra.Command{Use: "serve"},
			serveInputs(t, "", false, ""),
			resolvedinput.Inputs{},
		)
		if err == nil || !strings.Contains(err.Error(), "MCP stdio initializer is required") {
			t.Fatalf("handler error = %v, want missing initializer", err)
		}
	})
	t.Run("runtime home resolver", func(t *testing.T) {
		err := mcpcli.ResolvedServeHandler(mcpcli.ServeBinding{
			InitializeStdio: func(context.Context, startupcli.MCPIntent) error {
				t.Fatal("initializer must not run without a home resolver")
				return nil
			},
		})(
			&cobra.Command{Use: "serve"},
			serveInputs(t, "", true, "/workspace/project"),
			resolvedinput.Inputs{},
		)
		if err == nil || !strings.Contains(err.Error(), "process home directory resolver is required") {
			t.Fatalf("handler error = %v, want missing home resolver", err)
		}
	})
}

func TestResolvedServeHandler_FixtureAndRuntimePathsReachInitializer(t *testing.T) {
	t.Run("fixture-backed", func(t *testing.T) {
		want := errors.New("fixture initialize failed")
		handler := mcpcli.ResolvedServeHandler(mcpcli.ServeBinding{
			InitializeStdio: func(_ context.Context, intent startupcli.MCPIntent) error {
				if intent.RuntimeBacked || intent.FixtureCatalogPath != "fixtures.json" || intent.HomeDir != "" {
					t.Fatalf("fixture intent = %#v", intent)
				}
				return want
			},
		})
		cmd := &cobra.Command{Use: "serve"}
		cmd.SetIn(strings.NewReader(""))
		cmd.SetOut(io.Discard)
		err := handler(cmd, serveInputs(t, "fixtures.json", false, ""), resolvedinput.Inputs{})
		if !errors.Is(err, want) {
			t.Fatalf("handler error = %v, want initializer failure", err)
		}
	})
	t.Run("runtime-backed home failure", func(t *testing.T) {
		want := errors.New("home unavailable")
		handler := mcpcli.ResolvedServeHandler(mcpcli.ServeBinding{
			HomeDir: func() (string, error) { return "", want },
			InitializeStdio: func(context.Context, startupcli.MCPIntent) error {
				t.Fatal("initializer must not run after home failure")
				return nil
			},
		})
		err := handler(
			&cobra.Command{Use: "serve"},
			serveInputs(t, "", true, "/workspace/project"),
			resolvedinput.Inputs{},
		)
		if !errors.Is(err, want) || !strings.Contains(err.Error(), "resolve process home directory") {
			t.Fatalf("handler error = %v, want wrapped home failure", err)
		}
	})
	t.Run("runtime-backed success", func(t *testing.T) {
		var got startupcli.MCPIntent
		handler := mcpcli.ResolvedServeHandler(mcpcli.ServeBinding{
			HomeDir: func() (string, error) { return "/home/test", nil },
			InitializeStdio: func(_ context.Context, intent startupcli.MCPIntent) error {
				got = intent
				return nil
			},
		})
		cmd := &cobra.Command{Use: "serve"}
		stdin := strings.NewReader("request\n")
		var stdout bytes.Buffer
		cmd.SetIn(stdin)
		cmd.SetOut(&stdout)
		if err := handler(cmd, serveInputs(t, "", true, "/workspace/project"), resolvedinput.Inputs{}); err != nil {
			t.Fatalf("handler error = %v", err)
		}
		if !got.RuntimeBacked || got.HomeDir != "/home/test" || got.ProjectRoot != "/workspace/project" {
			t.Fatalf("runtime intent = %#v", got)
		}
		if got.Stdin != io.Reader(stdin) || got.Stdout != io.Writer(&stdout) {
			t.Fatalf("stdio streams were not forwarded")
		}
	})
}

func TestProcessMCPServe_RuntimeRejectsMissingHomeEnvironment(t *testing.T) {
	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "mcp", "serve", "--runtime", "--project-root", t.TempDir(),
	})
	inputs.Env = []string{"PATH="} // strip HOME/USERPROFILE so process home resolution fails
	err := process.Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), "home directory is not defined in the supplied environment") {
		t.Fatalf("Process.Execute(mcp serve --runtime) error = %v, want missing-home diagnostic", err)
	}
}

func partialServeInputs(
	t *testing.T,
	includeFixtureCatalog bool,
	includeRuntime bool,
	includeProjectRoot bool,
) resolvedinput.Inputs {
	t.Helper()
	definitions := make([]resolvedinput.Definition, 0, 3)
	candidates := make([]resolvedinput.Candidate, 0, 3)
	if includeFixtureCatalog {
		definitions = append(definitions, resolvedinput.Definition{
			ID: "you.mcp.serve.flag.fixture-catalog", Kind: resolvedinput.ValueKindString,
			Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault},
		})
		candidates = append(candidates, resolvedinput.Candidate{
			InputID: "you.mcp.serve.flag.fixture-catalog", Source: resolvedinput.SourceManifestDefault,
			Value: resolvedinput.StringValue("fixtures.json"),
		})
	}
	if includeRuntime {
		definitions = append(definitions, resolvedinput.Definition{
			ID: "you.mcp.serve.flag.runtime", Kind: resolvedinput.ValueKindBool,
			Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault},
		})
		candidates = append(candidates, resolvedinput.Candidate{
			InputID: "you.mcp.serve.flag.runtime", Source: resolvedinput.SourceManifestDefault,
			Value: resolvedinput.BoolValue(false),
		})
	}
	if includeProjectRoot {
		definitions = append(definitions, resolvedinput.Definition{
			ID: "you.mcp.serve.flag.project-root", Kind: resolvedinput.ValueKindString,
			Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault},
		})
		candidates = append(candidates, resolvedinput.Candidate{
			InputID: "you.mcp.serve.flag.project-root", Source: resolvedinput.SourceManifestDefault,
			Value: resolvedinput.StringValue("/workspace/project"),
		})
	}
	inputs, err := resolvedinput.Resolve(definitions, candidates)
	if err != nil {
		t.Fatalf("resolve partial serve inputs: %v", err)
	}
	return inputs
}

func serveInputs(
	t *testing.T,
	fixtureCatalog string,
	runtimeBacked bool,
	projectRoot string,
) resolvedinput.Inputs {
	t.Helper()
	definitions := []resolvedinput.Definition{
		{ID: "you.mcp.serve.flag.fixture-catalog", Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
		{ID: "you.mcp.serve.flag.runtime", Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
		{ID: "you.mcp.serve.flag.project-root", Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
	}
	inputs, err := resolvedinput.Resolve(definitions, []resolvedinput.Candidate{
		{InputID: "you.mcp.serve.flag.fixture-catalog", Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringValue(fixtureCatalog)},
		{InputID: "you.mcp.serve.flag.runtime", Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.BoolValue(runtimeBacked)},
		{InputID: "you.mcp.serve.flag.project-root", Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringValue(projectRoot)},
	})
	if err != nil {
		t.Fatalf("resolve serve inputs: %v", err)
	}
	return inputs
}

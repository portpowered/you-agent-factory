package mcpcli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestResolvedServeHandlerRequiresInjectedStdioInitializer(t *testing.T) {
	handler := ResolvedServeHandler(ServeBinding{})
	err := handler(&cobra.Command{Use: "serve"}, resolvedServeInputs(t, "", false, ""), resolvedinput.Inputs{})
	if err == nil || !strings.Contains(err.Error(), "MCP stdio initializer is required") {
		t.Fatalf("handler error = %v, want missing injected initializer", err)
	}
}

func TestResolvedServeHandlerDelegatesCanonicalIntentToStdioInitializer(t *testing.T) {
	stdin := strings.NewReader("request\n")
	var stdout bytes.Buffer
	var got startupcli.MCPIntent
	handler := ResolvedServeHandler(ServeBinding{
		HomeDir: func() (string, error) { return "/home/test", nil },
		InitializeStdio: func(_ context.Context, intent startupcli.MCPIntent) error {
			got = intent
			return nil
		},
	})
	cmd := &cobra.Command{Use: "serve"}
	cmd.SetIn(stdin)
	cmd.SetOut(&stdout)
	if err := handler(
		cmd,
		resolvedServeInputs(t, "fixtures.json", false, "/workspace/project"),
		resolvedinput.Inputs{},
	); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if got.FixtureCatalogPath != "fixtures.json" || got.RuntimeBacked || got.ProjectRoot != "/workspace/project" {
		t.Fatalf("stdio intent = %#v, want resolved fixture/runtime/project-root", got)
	}
	if got.Stdin != io.Reader(stdin) || got.Stdout != io.Writer(&stdout) {
		t.Fatalf("stdio = (%T, %T), want original command streams", got.Stdin, got.Stdout)
	}
}

func TestResolvedServeHandlerRuntimeCarriesInjectedHomeToInitializer(t *testing.T) {
	var got startupcli.MCPIntent
	handler := ResolvedServeHandler(ServeBinding{
		HomeDir: func() (string, error) { return "/home/test", nil },
		InitializeStdio: func(_ context.Context, intent startupcli.MCPIntent) error {
			got = intent
			return nil
		},
	})
	cmd := &cobra.Command{Use: "serve"}
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(io.Discard)
	if err := handler(
		cmd,
		resolvedServeInputs(t, "", true, "/workspace/project"),
		resolvedinput.Inputs{},
	); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if got.HomeDir != "/home/test" || got.ProjectRoot != "/workspace/project" || !got.RuntimeBacked {
		t.Fatalf("stdio intent = %#v, want injected home and runtime roots", got)
	}
}

func TestResolvedServeHandlerPreservesHomeResolutionFailure(t *testing.T) {
	want := errors.New("home unavailable")
	handler := ResolvedServeHandler(ServeBinding{
		HomeDir:         func() (string, error) { return "", want },
		InitializeStdio: func(context.Context, startupcli.MCPIntent) error { return nil },
	})
	err := handler(
		&cobra.Command{Use: "serve"},
		resolvedServeInputs(t, "", true, ""),
		resolvedinput.Inputs{},
	)
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "resolve process home directory") {
		t.Fatalf("handler error = %v, want wrapped home failure", err)
	}
}

func resolvedServeInputs(
	t *testing.T,
	fixtureCatalog string,
	runtimeBacked bool,
	projectRoot string,
) resolvedinput.Inputs {
	t.Helper()
	definitions := []resolvedinput.Definition{
		{ID: fixtureCatalogInputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
		{ID: runtimeInputID, Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
		{ID: projectRootInputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
	}
	inputs, err := resolvedinput.Resolve(definitions, []resolvedinput.Candidate{
		{InputID: fixtureCatalogInputID, Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringValue(fixtureCatalog)},
		{InputID: runtimeInputID, Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.BoolValue(runtimeBacked)},
		{InputID: projectRootInputID, Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringValue(projectRoot)},
	})
	if err != nil {
		t.Fatalf("resolve serve inputs: %v", err)
	}
	return inputs
}

package mcpcli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestResolvedServeHandlerRequiresInjectedStdioInitializer(t *testing.T) {
	err := ResolvedServeHandler(ServeBinding{})(
		&cobra.Command{Use: "serve"}, resolvedServeInputs(t, ""), resolvedinput.Inputs{},
	)
	if err == nil || !strings.Contains(err.Error(), "MCP stdio initializer is required") {
		t.Fatalf("handler error = %v, want missing injected initializer", err)
	}
}

func TestResolvedServeHandlerDelegatesProjectRootAndStreams(t *testing.T) {
	stdin := strings.NewReader("request\n")
	var stdout bytes.Buffer
	var got startupcli.MCPIntent
	handler := ResolvedServeHandler(ServeBinding{InitializeStdio: func(_ context.Context, intent startupcli.MCPIntent) error {
		got = intent
		return nil
	}})
	cmd := &cobra.Command{Use: "serve"}
	cmd.SetIn(stdin)
	cmd.SetOut(&stdout)
	if err := handler(cmd, resolvedServeInputs(t, "/workspace/project"), resolvedinput.Inputs{}); err != nil {
		t.Fatalf("serve: %v", err)
	}
	if got.ProjectRoot != "/workspace/project" || got.Stdin != stdin || got.Stdout != &stdout {
		t.Fatalf("MCP intent = %#v, want project root and invocation streams", got)
	}
}

func TestResolvedServeHandlerReportsMissingProjectRootInput(t *testing.T) {
	handler := ResolvedServeHandler(ServeBinding{InitializeStdio: func(context.Context, startupcli.MCPIntent) error {
		t.Fatal("initializer must not run before inputs resolve")
		return nil
	}})
	err := handler(&cobra.Command{Use: "serve"}, resolvedinput.Inputs{}, resolvedinput.Inputs{})
	if err == nil || !strings.Contains(err.Error(), "read MCP project root input") {
		t.Fatalf("handler error = %v, want missing canonical input", err)
	}
}

func TestResolvedServeHandlerPreservesInitializerFailure(t *testing.T) {
	want := errors.New("stdio initialize failed")
	handler := ResolvedServeHandler(ServeBinding{InitializeStdio: func(context.Context, startupcli.MCPIntent) error { return want }})
	err := handler(&cobra.Command{Use: "serve"}, resolvedServeInputs(t, ""), resolvedinput.Inputs{})
	if !errors.Is(err, want) {
		t.Fatalf("handler error = %v, want initializer failure", err)
	}
}

func resolvedServeInputs(t *testing.T, projectRoot string) resolvedinput.Inputs {
	t.Helper()
	inputs, err := resolvedinput.Resolve([]resolvedinput.Definition{{
		ID: projectRootInputID, Kind: resolvedinput.ValueKindString,
		Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault},
	}}, []resolvedinput.Candidate{{
		InputID: projectRootInputID, Source: resolvedinput.SourceManifestDefault,
		Value: resolvedinput.StringValue(projectRoot),
	}})
	if err != nil {
		t.Fatalf("resolve serve inputs: %v", err)
	}
	return inputs
}

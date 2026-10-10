package mcpcli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestResolvedServeHandlerRequiresInjectedStdioInitializer(t *testing.T) {
	err := ResolvedServeHandler(ServeBinding{})(
		&cobra.Command{Use: "serve"}, resolvedServeInputs(t, ""), resolvedServerInputs(t),
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
	if err := handler(cmd, resolvedServeInputs(t, "/workspace/project"), resolvedServerInputs(t)); err != nil {
		t.Fatalf("serve: %v", err)
	}
	if got.ProjectRoot != "/workspace/project" || got.ServerURL != "http://selected-host:7437" || got.Stdin != stdin || got.Stdout != &stdout {
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
	err := handler(&cobra.Command{Use: "serve"}, resolvedServeInputs(t, ""), resolvedServerInputs(t))
	if !errors.Is(err, want) {
		t.Fatalf("handler error = %v, want initializer failure", err)
	}
}

func TestResolvedServeHandlerReportsMissingServerInput(t *testing.T) {
	handler := ResolvedServeHandler(ServeBinding{InitializeStdio: func(context.Context, startupcli.MCPIntent) error {
		t.Fatal("initializer must not run before inherited server resolves")
		return nil
	}})
	err := handler(&cobra.Command{Use: "serve"}, resolvedServeInputs(t, ""), resolvedinput.Inputs{})
	if err == nil || !strings.Contains(err.Error(), "read MCP server input") {
		t.Fatalf("handler error = %v, want missing inherited server", err)
	}
}

func TestResolvedServeHandlerForwardsPrivateCallerAndRefusesMalformedPairs(t *testing.T) {
	t.Parallel()
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{47}, 32))
	for _, test := range []struct {
		name  string
		env   map[string]string
		valid bool
	}{
		{"running-shaped pair", map[string]string{"YOU_WORKER_SESSION_ID": "exact-caller", "YOU_WORKER_SESSION_TOKEN": token}, true},
		{"absent", nil, true},
		{"id only", map[string]string{"YOU_WORKER_SESSION_ID": "exact-caller"}, false},
		{"token only", map[string]string{"YOU_WORKER_SESSION_TOKEN": token}, false},
		{"empty pair", map[string]string{"YOU_WORKER_SESSION_ID": "", "YOU_WORKER_SESSION_TOKEN": ""}, false},
		{"invalid token", map[string]string{"YOU_WORKER_SESSION_ID": "exact-caller", "YOU_WORKER_SESSION_TOKEN": "planted-invalid-token"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			started := false
			handler := ResolvedServeHandler(ServeBinding{
				LookupEnv: func(key string) (string, bool) { value, present := test.env[key]; return value, present },
				InitializeStdio: func(_ context.Context, intent MCPIntent) error {
					started = true
					if intent.WorkerSessionID != test.env["YOU_WORKER_SESSION_ID"] || intent.WorkerSessionToken != test.env["YOU_WORKER_SESSION_TOKEN"] {
						t.Fatal("execution boundary changed caller credentials")
					}
					encoded, err := json.Marshal(intent)
					if err != nil || strings.Contains(string(encoded), token) || strings.Contains(string(encoded), "exact-caller") {
						t.Fatal("serialized lifecycle intent exposes credentials")
					}
					return nil
				},
			})
			err := handler(&cobra.Command{Use: "serve"}, resolvedServeInputs(t, ""), resolvedServerInputs(t))
			if started != test.valid || (err == nil) != test.valid {
				t.Fatalf("startup = %v, error = %v", started, err)
			}
			if err != nil && (!errors.Is(err, workersessions.ErrCallerInvalid) || !strings.Contains(err.Error(), "WORKER_SESSION_CALLER_INVALID") || strings.Contains(err.Error(), "planted-invalid-token") || strings.Contains(err.Error(), token)) {
				t.Fatal("malformed pair refusal is untyped or exposes credentials")
			}
		})
	}
}

func resolvedServerInputs(t *testing.T) resolvedinput.Inputs {
	t.Helper()
	inputs, err := resolvedinput.Resolve([]resolvedinput.Definition{{
		ID: "you.flag.server", Kind: resolvedinput.ValueKindString,
		Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag},
	}}, []resolvedinput.Candidate{{
		InputID: "you.flag.server", Source: resolvedinput.SourceCLIFlag,
		Value: resolvedinput.StringValue("http://selected-host:7437"),
	}})
	if err != nil {
		t.Fatalf("resolve inherited server: %v", err)
	}
	return inputs
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

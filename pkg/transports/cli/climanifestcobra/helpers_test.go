package climanifestcobra_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestSessionResolvedLifecyclePreservesTargetingOutputAndDiagnostics(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantPath    string
		response    string
		wantOutput  string
		diagnostics bool
	}{
		{
			name:        "default pause human",
			args:        []string{"--verbose", "session", "pause"},
			wantPath:    "/factory-sessions/~default/pause",
			response:    `{"sessionId":"~default","operation":"PAUSE","outcome":"ACCEPTED","status":"PAUSED"}`,
			wantOutput:  "Paused Factory session ~default (lifecycle status: PAUSED).",
			diagnostics: true,
		},
		{
			name:       "default resume human",
			args:       []string{"session", "resume"},
			wantPath:   "/factory-sessions/~default/resume",
			response:   `{"sessionId":"~default","operation":"RESUME","outcome":"ACCEPTED","status":"RUNNING"}`,
			wantOutput: "Resumed Factory session ~default (lifecycle status: RUNNING).",
		},
		{
			name:       "named live resume JSON",
			args:       []string{"--json", "session", "resume", "session-beta"},
			wantPath:   "/factory-sessions/session-beta/resume",
			response:   `{"sessionId":"session-beta","operation":"RESUME","outcome":"ACCEPTED","status":"RUNNING"}`,
			wantOutput: `"sessionId":"session-beta"`,
		},
		{
			name:       "durable pause human",
			args:       []string{"session", "pause", "dur-sess-review-001"},
			wantPath:   "/factory-sessions/dur-sess-review-001/pause",
			response:   `{"sessionId":"dur-sess-review-001","operation":"PAUSE","outcome":"ACCEPTED","status":"PAUSED"}`,
			wantOutput: "Paused Factory session dur-sess-review-001 (lifecycle status: PAUSED).",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			protocol := newSessionTestHTTPProtocol(t, func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodPost || request.URL.Path != test.wantPath {
					t.Fatalf("request = %s %s, want POST %s", request.Method, request.URL.Path, test.wantPath)
				}
				return sessionTestResponse(http.StatusOK, test.response), nil
			})
			services := commandregistry.SessionResolvedServices{
				PauseSession:  sessioncli.NewPause(protocol),
				ResumeSession: sessioncli.NewResume(protocol),
				Diagnostics:   func(cmd *cobra.Command) io.Writer { return cmd.ErrOrStderr() },
			}
			stdout, stderr, err := executeResolvedSessionWithOutput(t, services, test.args...)
			if err != nil {
				t.Fatalf("Execute(%v) error = %v", test.args, err)
			}
			if !strings.Contains(strings.TrimSpace(stdout), test.wantOutput) {
				t.Fatalf("Execute(%v) stdout = %q, want %q", test.args, stdout, test.wantOutput)
			}
			if test.diagnostics &&
				(!strings.Contains(stderr, "session pause request") ||
					!strings.Contains(stderr, "session pause response")) {
				t.Fatalf("Execute(%v) stderr = %q, want request and response diagnostics", test.args, stderr)
			}
		})
	}
}

func TestSessionResolvedLifecycleRejectsCardinalityAndPreservesFailures(t *testing.T) {
	calls := 0
	services := commandregistry.SessionResolvedServices{
		PauseSession: func(sessioncli.LifecycleControlConfig) error {
			calls++
			return nil
		},
		ResumeSession: func(sessioncli.LifecycleControlConfig) error {
			calls++
			return nil
		},
	}
	for _, operation := range []string{"pause", "resume"} {
		stdout, _, err := executeResolvedSessionWithOutput(
			t, services, "session", operation, "session-alpha", "session-beta",
		)
		if err == nil {
			t.Fatalf("%s with extra session ID error = nil", operation)
		}
		if stdout != "" {
			t.Fatalf("invalid %s stdout = %q, want empty", operation, stdout)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid lifecycle operation calls = %d, want 0", calls)
	}

	operationFailure := errors.New("lifecycle unavailable")
	for _, test := range []struct {
		name     string
		args     []string
		services commandregistry.SessionResolvedServices
		want     error
	}{
		{
			name: "pause failure", args: []string{"session", "pause", "session-alpha"},
			services: commandregistry.SessionResolvedServices{
				PauseSession: func(sessioncli.LifecycleControlConfig) error { return operationFailure },
			},
			want: operationFailure,
		},
		{
			name: "resume cancellation", args: []string{"session", "resume", "dur-sess-review-001"},
			services: commandregistry.SessionResolvedServices{
				ResumeSession: func(sessioncli.LifecycleControlConfig) error { return context.Canceled },
			},
			want: context.Canceled,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stdout, _, err := executeResolvedSessionWithOutput(t, test.services, test.args...)
			if !errors.Is(err, test.want) {
				t.Fatalf("Execute() error = %v, want %v", err, test.want)
			}
			if stdout != "" {
				t.Fatalf("Execute() stdout = %q, want empty", stdout)
			}
		})
	}
}

func TestSessionFamilyCommandExecutesManifestBoundLeaf(t *testing.T) {
	manifest := mustSessionManifest(t)
	calls := 0
	registry := sessionHandlerIDRegistry(t, manifest, func(cmd *cobra.Command, args []string) error {
		calls++
		if cmd.Name() != "show" {
			t.Fatalf("executed command = %q, want show", cmd.Name())
		}
		if len(args) != 1 || args[0] != "session-alpha" {
			t.Fatalf("handler args = %#v, want [session-alpha]", args)
		}
		return nil
	})

	root, err := climanifestcobra.NewSessionFamilyCommandFromManifest(manifest, registry)
	if err != nil {
		t.Fatalf("NewSessionFamilyCommandFromManifest() error = %v", err)
	}
	for _, leaf := range []string{
		"create", "delete", "list", "show", "dispatches", "pause", "resume",
	} {
		command, _, findErr := root.Find([]string{"session", leaf})
		if findErr != nil {
			t.Fatalf("Find(session %s) error = %v", leaf, findErr)
		}
		if command == nil || command.RunE == nil {
			t.Fatalf("session %s is not executable", leaf)
		}
	}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "show", "session-alpha"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("operation calls = %d, want 1", calls)
	}
}

func TestSessionFamilyCommandRejectsInvalidBindingsBeforeExecution(t *testing.T) {
	tests := []struct {
		name        string
		build       func(*testing.T, climanifest.Manifest, commandregistry.RunE) (*commandregistry.Registry, climanifest.Manifest)
		want        string
		nilRegistry bool
	}{
		{name: "nil registry", nilRegistry: true, want: "registry is required"},
		{
			name: "missing handler",
			build: func(t *testing.T, manifest climanifest.Manifest, operation commandregistry.RunE) (*commandregistry.Registry, climanifest.Manifest) {
				return sessionHandlerIDRegistryExcept(t, manifest, "you.session.create.handler", operation), manifest
			},
			want: "you.session.create.handler",
		},
		{
			name: "cross-family handler",
			build: func(t *testing.T, manifest climanifest.Manifest, operation commandregistry.RunE) (*commandregistry.Registry, climanifest.Manifest) {
				registry := sessionHandlerIDRegistry(t, manifest, operation)
				if err := registry.Register("you.work.list.handler", noOpSessionHandler); err != nil {
					t.Fatalf("Register(cross-family) error = %v", err)
				}
				return registry, manifest
			},
			want: "you.work.list.handler",
		},
		{
			name: "duplicate manifest handler",
			build: func(t *testing.T, manifest climanifest.Manifest, operation commandregistry.RunE) (*commandregistry.Registry, climanifest.Manifest) {
				registry := sessionHandlerIDRegistry(t, manifest, operation)
				create := manifest.Commands["you.session.create"]
				deleteCommand := manifest.Commands["you.session.delete"]
				deleteCommand.Handler.ID = create.Handler.ID
				manifest.Commands[deleteCommand.ID] = deleteCommand
				return registry, manifest
			},
			want: "duplicated",
		},
		{
			name: "unbound manifest handler",
			build: func(t *testing.T, manifest climanifest.Manifest, operation commandregistry.RunE) (*commandregistry.Registry, climanifest.Manifest) {
				registry := sessionHandlerIDRegistry(t, manifest, operation)
				show := manifest.Commands["you.session.show"]
				show.Handler.ID = "you.session.show.replacement-handler"
				manifest.Commands[show.ID] = show
				return registry, manifest
			},
			want: "replacement-handler",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := mustSessionManifest(t)
			calls := 0
			var registry *commandregistry.Registry
			if !test.nilRegistry {
				operation := func(*cobra.Command, []string) error {
					calls++
					return nil
				}
				registry, manifest = test.build(t, manifest, operation)
			}
			_, err := climanifestcobra.NewSessionFamilyCommandFromManifest(manifest, registry)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewSessionFamilyCommandFromManifest() error = %v, want %q", err, test.want)
			}
			if calls != 0 {
				t.Fatalf("operation calls = %d, want 0", calls)
			}
		})
	}
}

func mustSessionManifest(t *testing.T) climanifest.Manifest {
	t.Helper()
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		t.Fatalf("SessionFamilyManifest() error = %v", err)
	}
	return manifest
}

func mustSessionHandlerIDs(t *testing.T, manifest climanifest.Manifest) []string {
	t.Helper()
	handlerIDs, err := commandregistry.RunnableSessionHandlerIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableSessionHandlerIDs() error = %v", err)
	}
	return handlerIDs
}

func sessionHandlerIDRegistry(
	t *testing.T,
	manifest climanifest.Manifest,
	handler commandregistry.RunE,
) *commandregistry.Registry {
	t.Helper()
	registry := commandregistry.NewRegistry()
	for _, handlerID := range mustSessionHandlerIDs(t, manifest) {
		if handler == nil {
			handler = noOpSessionHandler
		}
		if err := registry.Register(handlerID, handler); err != nil {
			t.Fatalf("Register(%q) error = %v", handlerID, err)
		}
	}
	return registry
}

func sessionHandlerIDRegistryExcept(
	t *testing.T,
	manifest climanifest.Manifest,
	excluded string,
	handler commandregistry.RunE,
) *commandregistry.Registry {
	t.Helper()
	registry := commandregistry.NewRegistry()
	for _, handlerID := range mustSessionHandlerIDs(t, manifest) {
		if handlerID == excluded {
			continue
		}
		if err := registry.Register(handlerID, handler); err != nil {
			t.Fatalf("Register(%q) error = %v", handlerID, err)
		}
	}
	return registry
}

func noOpSessionHandler(*cobra.Command, []string) error {
	return nil
}

func TestRunServerExecutableSurfaceMatchesGeneratedManifest(t *testing.T) {
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		t.Fatalf("RunSubmitFamilyManifest() error = %v", err)
	}
	components := mustRunSubmitFamilyComponents(t)
	runRecord := manifest.Commands["you.run"]
	serverRecord := manifest.Commands["you.server"]

	assertRunServerCommandMetadata(t, components, runRecord, serverRecord)
	assertRunFlagParity(t, components, runRecord)
	assertRunServerFailureMetadata(t, runRecord, serverRecord)
}

func assertRunServerCommandMetadata(
	t *testing.T,
	components climanifestcobra.RunSubmitFamilyComponents,
	runRecord, serverRecord climanifest.Command,
) {
	t.Helper()
	if components.Run.Use != runRecord.Usage.Line ||
		components.Run.Short != runRecord.Documentation.Documentation.Title.CanonicalEnglish {
		t.Fatal("generated run command metadata drifted from the manifest")
	}
	if components.Server.Use != serverRecord.Usage.Line ||
		components.Server.Short != serverRecord.Documentation.Documentation.Title.CanonicalEnglish {
		t.Fatal("generated server command metadata drifted from the manifest")
	}
}

func assertRunFlagParity(
	t *testing.T,
	components climanifestcobra.RunSubmitFamilyComponents,
	runRecord climanifest.Command,
) {
	t.Helper()
	for _, flag := range runRecord.Flags {
		if flag.Scope != "local" {
			continue
		}
		registered := components.Run.Flags().Lookup(flag.Long)
		if registered == nil || registered.Shorthand != flag.Shorthand ||
			registered.DefValue != flag.Default || registered.Usage != flag.Usage {
			t.Fatalf("run flag %q = %#v, want manifest parity", flag.ID, registered)
		}
	}
	named := components.Run.Flags().Lookup("named")
	if named == nil || named.Shorthand != "a" || components.Run.Flags().Lookup("a") != nil {
		t.Fatalf("canonical named selector = %#v, want --named with -a and no --a", named)
	}
}

func assertRunServerFailureMetadata(
	t *testing.T,
	runRecord, serverRecord climanifest.Command,
) {
	t.Helper()
	if len(runRecord.Errors) == 0 || len(serverRecord.Errors) == 0 ||
		runRecord.Exits["you.run.exit.cancel"].Code != 130 ||
		serverRecord.Exits["you.server.exit.cancel"].Code != 130 {
		t.Fatal("run/server symbolic errors and cancellation exits are incomplete")
	}
}

func TestNewCommandTreeRejectsInvalidCanonicalFlagValuesBeforeDispatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*climanifest.Flag)
		want   string
	}{
		{name: "default", mutate: func(flag *climanifest.Flag) {
			value := []string{" Base "}
			flag.DefaultValue = &climanifest.InputValue{StringArray: &value}
			flag.Normalization = "lowercase-trim"
		}, want: "typed default is not in declared normalized form"},
		{name: "choice", mutate: func(flag *climanifest.Flag) {
			flag.Enum = []string{"BASE"}
			flag.Normalization = "lowercase"
		}, want: "enumerated choice"},
		{name: "cardinality range", mutate: func(flag *climanifest.Flag) {
			flag.Required = true
			flag.MinCardinality = 2
			flag.MaxCardinality = 1
			flag.Repeatable = false
		}, want: "invalid cardinality"},
		{name: "default cardinality", mutate: func(flag *climanifest.Flag) {
			flag.Required = true
			flag.MinCardinality = 2
		}, want: "typed default count is outside declared cardinality"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := syntheticFlagManifest()
			updateSyntheticFlag(&manifest, "stable.alpha", "stable.alpha.flag.tags", test.mutate)
			calls := 0
			bindings := genericBindingsForManifest(manifest)
			for id := range bindings.Handlers {
				bindings.Handlers[id] = func(context.Context, map[string]any) error {
					calls++
					return nil
				}
			}
			root, err := climanifestcobra.NewCommandTree(manifest, bindings)
			if root != nil || err == nil || !strings.Contains(err.Error(), test.want) || calls != 0 {
				t.Fatalf("NewCommandTree() = (%v, %v), calls=%d; want nil, %q, zero", root, err, calls, test.want)
			}
		})
	}
}

func findCommandByPath(root *cobra.Command, path string) (*cobra.Command, error) {
	parts := strings.Fields(path)
	if len(parts) == 0 || parts[0] != root.Name() {
		return nil, fmt.Errorf("path %q does not start at root %q", path, root.Name())
	}

	current := root
	for _, segment := range parts[1:] {
		found := false
		for _, child := range current.Commands() {
			if child.Name() == segment {
				current = child
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("command segment %q not found under %q", segment, current.CommandPath())
		}
	}
	return current, nil
}

func withActiveFlagLifecycle(flags map[string]climanifest.Flag) {
	for id, flag := range flags {
		flag.Lifecycle = activeLifecycle(id)
		if flag.Completion == "" {
			flag.Completion = "none"
		}
		if flag.DefaultValue != nil || flag.NoOptionValue != nil || flag.Kind != "" {
			flag.Kind = "named"
			flag.MinCardinality = 0
			if flag.Required {
				flag.MinCardinality = 1
			}
			flag.MaxCardinality = 1
			if flag.Repeatable {
				flag.MaxCardinality = -1
			}
			flag.AcceptedSources = []string{"cli"}
			if flag.DefaultValue != nil {
				flag.AcceptedSources = append(flag.AcceptedSources, "manifest-default")
			}
			flag.HandlerBindingID = flag.ID + ".binding"
			if flag.Scope == "inherited" {
				flag.HandlerBindingID = flag.InheritedFromID + ".binding"
			}
		}
		flags[id] = flag
	}
}

func withNoneArgumentCompletion(arguments map[string]climanifest.Argument) {
	for id, argument := range arguments {
		argument.Completion = "none"
		argument.DoubleDash = "terminates-flags"
		if argument.DefaultValue == nil {
			argument.Channels = []string{"cli"}
		} else {
			argument = canonicalTestArgument(argument)
		}
		arguments[id] = argument
	}
}

func canonicalTestArgument(argument climanifest.Argument) climanifest.Argument {
	argument.Channels = nil
	argument.Scope = "local"
	argument.AcceptedSources = []string{"cli"}
	if argument.DefaultValue != nil {
		argument.AcceptedSources = append(argument.AcceptedSources, "manifest-default")
	}
	argument.HandlerBindingID = argument.ID + ".binding"
	argument.Visibility = "visible"
	argument.Lifecycle = activeLifecycle(argument.ID)
	return argument
}

func completeCanonicalCommandContract(command *climanifest.Command) {
	command.HandlerBindings = make(map[string]climanifest.HandlerBinding)
	command.SourceBindings = make(map[string]climanifest.SourceBinding)
	canonical := false
	add := func(id, bindingID, valueType string, sources []string) {
		if bindingID == "" {
			return
		}
		canonical = true
		command.HandlerBindings[bindingID] = climanifest.HandlerBinding{ID: bindingID, InputID: id}
		for _, source := range sources {
			if source != "stdin" && source != "environment" && source != "operator-config" {
				continue
			}
			bindingID := id + ".source." + source
			externalKey := ""
			if source != "stdin" {
				externalKey = strings.NewReplacer(".", "_", "-", "_").Replace(strings.ToUpper(id))
			}
			command.SourceBindings[bindingID] = climanifest.SourceBinding{
				ID: bindingID, Source: source, ExternalKey: externalKey, InputID: id,
			}
		}
	}
	for _, argument := range command.Arguments {
		add(argument.ID, argument.HandlerBindingID, argument.ValueType, argument.AcceptedSources)
	}
	for _, flag := range command.Flags {
		add(flag.ID, flag.HandlerBindingID, flag.ValueType, flag.AcceptedSources)
	}
	if !canonical {
		command.HandlerBindings = nil
		command.SourceBindings = nil
		return
	}
	command.Precedence = climanifest.Precedence{
		Order: []string{
			"cli", "stdin", "environment", "operator-config",
			"manifest-default", "factory-signature-default",
		},
		WithinTier:       climanifest.WithinTierRule{Scalar: "last", Repeated: "append"},
		AcrossTiers:      "replace",
		MultipleBindings: "reject",
	}
}

func TestNewCommandTreeResolvesPersistentInputsForDescendants(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		environment string
		want        bool
		source      resolvedinput.Source
		changed     bool
	}{
		{
			name:   "manifest default at deep descendant",
			args:   []string{"alpha", "leaf"},
			source: resolvedinput.SourceManifestDefault,
		},
		{
			name:    "long spelling before descendant",
			args:    []string{"--activate", "alpha", "leaf"},
			want:    true,
			source:  resolvedinput.SourceCLIFlag,
			changed: true,
		},
		{
			name:    "alias after descendant",
			args:    []string{"alpha", "leaf", "--enable"},
			want:    true,
			source:  resolvedinput.SourceCLIFlag,
			changed: true,
		},
		{
			name:    "shorthand after parent",
			args:    []string{"alpha", "-a", "leaf"},
			want:    true,
			source:  resolvedinput.SourceCLIFlag,
			changed: true,
		},
		{
			name:        "CLI precedes configured source",
			args:        []string{"alpha", "leaf", "--activate=false"},
			environment: "true",
			source:      resolvedinput.SourceCLIFlag,
			changed:     true,
		},
		{
			name:        "configured source precedes default",
			args:        []string{"alpha", "leaf"},
			environment: "true",
			want:        true,
			source:      resolvedinput.SourceEnvironment,
			changed:     true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := persistentResolutionManifest()
			var received resolvedinput.Inputs
			bindings := genericBindingsForManifest(manifest)
			bindings.Handlers["stable.leaf.handler"] = func(
				ctx context.Context,
				_ map[string]any,
			) error {
				var err error
				received, err = climanifestcobra.ResolvedPersistentInputsFromContext(ctx)
				return err
			}
			bindings.SourceValues = func(
				_ context.Context,
				_ climanifest.SourceBinding,
				_ resolvedinput.ValueKind,
			) (resolvedinput.Value, bool, error) {
				if test.environment == "" {
					return resolvedinput.Value{}, false, nil
				}
				return resolvedinput.BoolValue(test.environment == "true"), true, nil
			}
			root, err := climanifestcobra.NewCommandTree(manifest, bindings)
			if err != nil {
				t.Fatalf("NewCommandTree() error = %v", err)
			}
			root.SetArgs(test.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			value, err := received.Bool("stable.root.flag.activate")
			if err != nil || value != test.want {
				t.Fatalf("resolved value = (%t, %v), want %t", value, err, test.want)
			}
			state, ok := received.State("stable.root.flag.activate")
			if !ok || state.Provenance != test.source || state.Changed != test.changed ||
				state.Default != (test.source == resolvedinput.SourceManifestDefault) {
				t.Fatalf("resolved state = (%#v, %t), want source=%q changed=%t", state, ok, test.source, test.changed)
			}
			observation, ok := received.Observe("stable.root.flag.activate")
			if !ok || observation.Value != resolvedinput.RedactedValue ||
				observation.Provenance != test.source || observation.Changed != test.changed {
				t.Fatalf("redacted observation = (%#v, %t)", observation, ok)
			}
		})
	}
}

func persistentResolutionManifest() climanifest.Manifest {
	manifest := syntheticFlagManifest()
	root := manifest.Commands["stable.root"]
	flag := root.Flags["stable.root.flag.activate"]
	flag.AcceptedSources = []string{
		climanifest.SourceCLI,
		climanifest.SourceEnvironment,
		climanifest.SourceManifestDefault,
	}
	flag.HandlerBindingID = "stable.root.binding.activate"
	flag.Sensitivity = "sensitive"
	root.Flags[flag.ID] = flag
	root.HandlerBindings = map[string]climanifest.HandlerBinding{
		flag.HandlerBindingID: {ID: flag.HandlerBindingID, InputID: flag.ID},
	}
	root.SourceBindings = map[string]climanifest.SourceBinding{
		"stable.root.source.activate.environment": {
			ID:          "stable.root.source.activate.environment",
			Source:      climanifest.SourceEnvironment,
			ExternalKey: "FORGE_ACTIVATE",
			InputID:     flag.ID,
		},
	}
	root.Precedence = climanifest.CanonicalPrecedence()
	manifest.Commands[root.ID] = root
	leaf := syntheticCommand("stable.leaf", "leaf", "forge alpha leaf", true)
	manifest.Commands[leaf.ID] = leaf
	return manifest
}

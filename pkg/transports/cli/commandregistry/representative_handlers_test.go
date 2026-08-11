package commandregistry_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func noopRunE(*cobra.Command, []string) error { return nil }

func TestVerifySessionHandlerIDCoverageRequiresExactManifestBindings(t *testing.T) {
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		t.Fatalf("SessionFamilyManifest() error = %v", err)
	}
	handlerIDs, err := commandregistry.RunnableSessionHandlerIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableSessionHandlerIDs() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	for _, handlerID := range handlerIDs {
		if err := registry.Register(handlerID, noopRunE); err != nil {
			t.Fatalf("Register(%q) error = %v", handlerID, err)
		}
	}
	if err := registry.VerifySessionHandlerIDCoverage(manifest); err != nil {
		t.Fatalf("VerifySessionHandlerIDCoverage() error = %v", err)
	}

	if err := registry.Register("you.work.list.handler", noopRunE); err != nil {
		t.Fatalf("Register(cross-family) error = %v", err)
	}
	err = registry.VerifySessionHandlerIDCoverage(manifest)
	if err == nil || !strings.Contains(err.Error(), "you.work.list.handler") {
		t.Fatalf("VerifySessionHandlerIDCoverage() error = %v, want cross-family binding", err)
	}
}

func TestRunnableSessionHandlerIDsRejectsDuplicateManifestBinding(t *testing.T) {
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		t.Fatalf("SessionFamilyManifest() error = %v", err)
	}
	create := manifest.Commands["you.session.create"]
	deleteCommand := manifest.Commands["you.session.delete"]
	deleteCommand.Handler.ID = create.Handler.ID
	manifest.Commands[deleteCommand.ID] = deleteCommand

	_, err = commandregistry.RunnableSessionHandlerIDs(manifest)
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("RunnableSessionHandlerIDs() error = %v, want duplicate handler ID", err)
	}
}

func TestRunnableRepresentativeCommandIDsFromGeneratedManifest(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	ids, err := commandregistry.RunnableRepresentativeCommandIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableRepresentativeCommandIDs() error = %v", err)
	}
	if len(ids) != 2 || ids[0] != "you" || ids[1] != "you.session.show" {
		t.Fatalf("runnable IDs = %#v, want [you you.session.show]", ids)
	}
}

func TestVerifyRepresentativeRunnableCoverage(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you.session.show", noopRunE); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.VerifyRepresentativeRunnableCoverage(manifest); err == nil {
		t.Fatal("missing root handler = nil, want error")
	}
	if err := registry.Register("you", noopRunE); err != nil {
		t.Fatalf("Register(you) error = %v", err)
	}
	if err := registry.VerifyRepresentativeRunnableCoverage(manifest); err != nil {
		t.Fatalf("complete coverage error = %v", err)
	}
}

func TestNewRepresentativeRegistryRegistersContractedRunnableIDs(t *testing.T) {
	registry, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewRepresentativeRegistry() error = %v", err)
	}
	if _, lookupErr := registry.Lookup("you"); lookupErr != nil {
		t.Fatalf("Lookup(you) error = %v", lookupErr)
	}
}

func TestNewSessionResolvedRegistryRejectsInvalidManifestBindings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]climanifest.Command)
		want   string
	}{
		{
			name: "missing command",
			mutate: func(commands map[string]climanifest.Command) {
				delete(commands, "you.session.create")
			},
			want: "you.session.create",
		},
		{
			name: "missing handler",
			mutate: func(commands map[string]climanifest.Command) {
				record := commands["you.session.create"]
				record.Handler = nil
				commands[record.ID] = record
			},
			want: "has no handler ID",
		},
		{
			name: "duplicate handler",
			mutate: func(commands map[string]climanifest.Command) {
				record := commands["you.session.create"]
				record.Handler.ID = commands["you.session.delete"].Handler.ID
				commands[record.ID] = record
			},
			want: "duplicate handler registration",
		},
		{
			name: "extra command",
			mutate: func(commands map[string]climanifest.Command) {
				commands["foreign"] = commands["you.session"]
			},
			want: "command count",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest, err := generated.SessionFamilyManifest()
			if err != nil {
				t.Fatalf("SessionFamilyManifest() error = %v", err)
			}
			test.mutate(manifest.Commands)
			if _, err := commandregistry.NewSessionResolvedRegistry(
				manifest,
				commandregistry.SessionResolvedServices{},
			); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewSessionResolvedRegistry() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSessionResolvedHandlersRejectMissingOperations(t *testing.T) {
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		t.Fatalf("SessionFamilyManifest() error = %v", err)
	}
	registry, err := commandregistry.NewSessionResolvedRegistry(
		manifest,
		commandregistry.SessionResolvedServices{},
	)
	if err != nil {
		t.Fatalf("NewSessionResolvedRegistry() error = %v", err)
	}
	handlerIDs, err := commandregistry.RunnableSessionHandlerIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableSessionHandlerIDs() error = %v", err)
	}
	for _, handlerID := range handlerIDs {
		handlers, err := registry.LookupHandlers(handlerID)
		if err != nil {
			t.Fatalf("LookupHandlers(%q) error = %v", handlerID, err)
		}
		err = handlers.ResolvedRunE(
			&cobra.Command{Use: handlerID},
			resolvedinput.Inputs{},
			resolvedinput.Inputs{},
		)
		if err == nil {
			t.Fatalf("resolved handler %q missing operation error = nil", handlerID)
		}
	}

	registry, err = commandregistry.NewSessionResolvedRegistry(
		manifest,
		commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
			Create:         func(sessioncli.CreateConfig) error { return nil },
			Delete:         func(sessioncli.DeleteConfig) error { return nil },
			List:           func(sessioncli.ListConfig) error { return nil },
			Show:           func(sessioncli.ShowConfig) error { return nil },
			ListDispatches: func(sessioncli.DispatchesConfig) error { return nil },
			Pause:          func(sessioncli.LifecycleControlConfig) error { return nil },
			Resume:         func(sessioncli.LifecycleControlConfig) error { return nil },
		}, nil, nil),
	)
	if err != nil {
		t.Fatalf("NewSessionResolvedRegistry(valid services) error = %v", err)
	}
	for _, handlerID := range handlerIDs {
		handlers, err := registry.LookupHandlers(handlerID)
		if err != nil {
			t.Fatalf("LookupHandlers(%q) error = %v", handlerID, err)
		}
		err = handlers.ResolvedRunE(
			&cobra.Command{Use: handlerID},
			resolvedinput.Inputs{},
			resolvedinput.Inputs{},
		)
		if err == nil {
			t.Fatalf("resolved handler %q missing input error = nil", handlerID)
		}
	}
}

func TestSessionLifecycleHandlersSelectRequestedPlacement(t *testing.T) {
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		t.Fatalf("SessionFamilyManifest() error = %v", err)
	}

	type lifecycleCall struct {
		placement string
		operation string
		config    sessioncli.LifecycleControlConfig
	}
	var calls []lifecycleCall
	service := func(placement string) sessioncli.Service {
		capture := func(operation string) func(sessioncli.LifecycleControlConfig) error {
			return func(config sessioncli.LifecycleControlConfig) error {
				calls = append(calls, lifecycleCall{placement: placement, operation: operation, config: config})
				return nil
			}
		}
		return sessioncli.Bind(sessioncli.Operations{
			Pause:     capture("pause"),
			Resume:    capture("resume"),
			Cancel:    capture("cancel"),
			Terminate: capture("terminate"),
		})
	}
	registry, err := commandregistry.NewSessionResolvedRegistry(manifest, commandregistry.SessionResolvedServices{
		LocalSessions:  service("local"),
		RemoteSessions: service("remote"),
	})
	if err != nil {
		t.Fatalf("NewSessionResolvedRegistry() error = %v", err)
	}

	tests := []struct {
		name       string
		handlerID  string
		inputID    string
		operation  string
		remote     bool
		wantServer string
	}{
		{name: "local pause", handlerID: "you.session.pause.handler", inputID: "you.session.pause.arg.0", operation: "pause", wantServer: "http://local.test"},
		{name: "remote resume", handlerID: "you.session.resume.handler", inputID: "you.session.resume.arg.0", operation: "resume", remote: true, wantServer: "http://remote.test"},
		{name: "local cancel", handlerID: "you.session.cancel.handler", inputID: "you.session.cancel.arg.0", operation: "cancel", wantServer: "http://local.test"},
		{name: "remote terminate", handlerID: "you.session.terminate.handler", inputID: "you.session.terminate.arg.0", operation: "terminate", remote: true, wantServer: "http://remote.test"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handlers, err := registry.LookupHandlers(test.handlerID)
			if err != nil {
				t.Fatalf("LookupHandlers(%q) error = %v", test.handlerID, err)
			}
			inputs := resolvedTestInputs(t,
				resolvedTestValue{id: test.inputID, source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("dur-session-placement-001")},
			)
			inherited := resolvedTestInputs(t,
				resolvedTestValue{id: "you.flag.server", source: resolvedinput.SourceManifestDefault, value: resolvedinput.StringValue(test.wantServer)},
				resolvedTestValue{id: "you.flag.json", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
				resolvedTestValue{id: "you.flag.remote", source: resolvedinput.SourceCLIFlag, value: resolvedinput.BoolValue(test.remote)},
				resolvedTestValue{id: "you.flag.verbose", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
				resolvedTestValue{id: "you.flag.debug", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
			)
			var output bytes.Buffer
			cmd := &cobra.Command{Use: test.name}
			cmd.SetContext(context.Background())
			cmd.SetOut(&output)
			if err := handlers.ResolvedRunE(cmd, inputs, inherited); err != nil {
				t.Fatalf("resolved lifecycle handler: %v", err)
			}
			if len(calls) == 0 {
				t.Fatal("lifecycle operation was not called")
			}
			got := calls[len(calls)-1]
			wantPlacement := "local"
			if test.remote {
				wantPlacement = "remote"
			}
			if got.placement != wantPlacement || got.operation != test.operation {
				t.Fatalf("lifecycle call = %#v, want placement=%q operation=%q", got, wantPlacement, test.operation)
			}
			if got.config.SessionID != "dur-session-placement-001" || got.config.Server != test.wantServer {
				t.Fatalf("lifecycle config = %#v, want exact session/server", got.config)
			}
		})
	}
}

func TestSessionLifecyclePlacementFallbackAndMissingServiceAreExplicit(t *testing.T) {
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		t.Fatalf("SessionFamilyManifest() error = %v", err)
	}
	inputs := resolvedTestInputs(t,
		resolvedTestValue{id: "you.session.pause.arg.0", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("dur-session-placement-002")},
	)
	globalsWithoutRemote := resolvedTestInputs(t,
		resolvedTestValue{id: "you.flag.server", source: resolvedinput.SourceManifestDefault, value: resolvedinput.StringValue("http://compat.test")},
		resolvedTestValue{id: "you.flag.json", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
		resolvedTestValue{id: "you.flag.verbose", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
		resolvedTestValue{id: "you.flag.debug", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
	)
	called := false
	compatibility := sessioncli.Bind(sessioncli.Operations{
		Pause: func(config sessioncli.LifecycleControlConfig) error {
			called = config.SessionID == "dur-session-placement-002"
			return nil
		},
	})
	registry, err := commandregistry.NewSessionResolvedRegistry(manifest, commandregistry.SessionResolvedServices{LocalSessions: compatibility})
	if err != nil {
		t.Fatalf("NewSessionResolvedRegistry(explicit local service) error = %v", err)
	}
	handlers, err := registry.LookupHandlers("you.session.pause.handler")
	if err != nil {
		t.Fatalf("LookupHandlers() error = %v", err)
	}
	if err := handlers.ResolvedRunE(&cobra.Command{Use: "pause"}, inputs, globalsWithoutRemote); err != nil {
		t.Fatalf("explicit local placement handler: %v", err)
	}
	if !called {
		t.Fatal("explicit local service did not receive local pause")
	}
	missingRegistry, err := commandregistry.NewSessionResolvedRegistry(manifest, commandregistry.SessionResolvedServices{})
	if err != nil {
		t.Fatalf("NewSessionResolvedRegistry(missing) error = %v", err)
	}
	for _, test := range []struct {
		name   string
		remote bool
		want   string
	}{
		{name: "local", want: "session pause service is required for local placement"},
		{name: "remote", remote: true, want: "session pause service is required for remote placement"},
	} {
		t.Run(test.name, func(t *testing.T) {
			inherited := globalsWithoutRemote
			if test.remote {
				inherited = resolvedTestInputs(t,
					resolvedTestValue{id: "you.flag.server", source: resolvedinput.SourceManifestDefault, value: resolvedinput.StringValue("http://missing.test")},
					resolvedTestValue{id: "you.flag.json", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
					resolvedTestValue{id: "you.flag.remote", source: resolvedinput.SourceCLIFlag, value: resolvedinput.BoolValue(true)},
					resolvedTestValue{id: "you.flag.verbose", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
					resolvedTestValue{id: "you.flag.debug", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
				)
			}
			handlers, lookupErr := missingRegistry.LookupHandlers("you.session.pause.handler")
			if lookupErr != nil {
				t.Fatalf("LookupHandlers() error = %v", lookupErr)
			}
			err := handlers.ResolvedRunE(&cobra.Command{Use: test.name}, inputs, inherited)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("placement error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSessionLifecycleRemotePlacementDoesNotUseLocalFallback(t *testing.T) {
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		t.Fatalf("SessionFamilyManifest() error = %v", err)
	}
	inputs := resolvedTestInputs(t,
		resolvedTestValue{id: "you.session.pause.arg.0", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("dur-session-placement-002")},
	)
	compatibility := sessioncli.Bind(sessioncli.Operations{
		Pause: func(sessioncli.LifecycleControlConfig) error { return nil },
	})
	registry, err := commandregistry.NewSessionResolvedRegistry(manifest, commandregistry.SessionResolvedServices{LocalSessions: compatibility})
	if err != nil {
		t.Fatalf("NewSessionResolvedRegistry() error = %v", err)
	}
	handlers, err := registry.LookupHandlers("you.session.pause.handler")
	if err != nil {
		t.Fatalf("LookupHandlers() error = %v", err)
	}
	remoteGlobals := resolvedTestInputs(t,
		resolvedTestValue{id: "you.flag.server", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("http://remote-only.test")},
		resolvedTestValue{id: "you.flag.remote", source: resolvedinput.SourceCLIFlag, value: resolvedinput.BoolValue(true)},
		resolvedTestValue{id: "you.flag.json", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
		resolvedTestValue{id: "you.flag.verbose", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
		resolvedTestValue{id: "you.flag.debug", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
	)
	if err := handlers.ResolvedRunE(&cobra.Command{Use: "pause"}, inputs, remoteGlobals); err == nil || err.Error() != "session pause service is required for remote placement" {
		t.Fatalf("remote placement with only local service error = %v, want explicit missing remote service", err)
	}
}

func TestSessionResourceSetResolvedHandlerMapsStableInputs(t *testing.T) {
	var got sessioncli.ResourceCapacityConfig
	var diagnostics bytes.Buffer
	handlers := commandregistry.BindSessionResolvedHandlers(
		commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
			SetResourceCapacity: func(cfg sessioncli.ResourceCapacityConfig) error {
				got = cfg
				return nil
			},
		}, nil, func(*cobra.Command) io.Writer { return &diagnostics }),
	)
	ctx := context.WithValue(context.Background(), struct{}{}, "session-resource-set")
	var output bytes.Buffer
	cmd := &cobra.Command{Use: "set"}
	cmd.SetContext(ctx)
	cmd.SetOut(&output)
	inputs := resolvedTestInputs(t,
		resolvedTestValue{id: "you.session.resource.set.arg.0", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("reviewers")},
		resolvedTestValue{id: "you.session.resource.set.arg.1", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.IntValue(8)},
		resolvedTestValue{id: "you.session.resource.set.arg.2", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("session-beta")},
		resolvedTestValue{id: "you.session.resource.set.flag.request-id", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("raise-reviewers-001")},
		resolvedTestValue{id: "you.session.resource.set.flag.expected-revision", source: resolvedinput.SourceCLIFlag, value: resolvedinput.IntValue(3)},
		resolvedTestValue{id: "you.session.resource.set.flag.reason", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("operator capacity increase")},
	)
	inherited := resolvedFactoryGlobals(t, true, false, true)
	if err := handlers.ResourceSet(cmd, inputs, inherited); err != nil {
		t.Fatalf("ResourceSet() error = %v", err)
	}
	if got.Context != ctx || got.Server != "http://localhost:7437" || got.SessionID != "session-beta" ||
		got.ResourceID != "reviewers" || got.Capacity != 8 || got.ExpectedRevision != 3 ||
		got.RequestID != "raise-reviewers-001" || got.Reason != "operator capacity increase" ||
		!got.JSON || !got.Debug || !got.Verbose || got.Output != &output {
		t.Fatalf("resource capacity config = %#v, want stable-input mapping", got)
	}
	if got.Diagnostics != &diagnostics {
		t.Fatalf("diagnostics writer = %T, want injected writer", got.Diagnostics)
	}

	base := []resolvedTestValue{
		{id: "you.session.resource.set.arg.0", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("reviewers")},
		{id: "you.session.resource.set.arg.1", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.IntValue(8)},
		{id: "you.session.resource.set.arg.2", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("session-beta")},
		{id: "you.session.resource.set.flag.request-id", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("raise-reviewers-001")},
		{id: "you.session.resource.set.flag.expected-revision", source: resolvedinput.SourceCLIFlag, value: resolvedinput.IntValue(3)},
		{id: "you.session.resource.set.flag.reason", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("operator capacity increase")},
	}
	for _, omitted := range []string{
		"you.session.resource.set.arg.0",
		"you.session.resource.set.arg.1",
		"you.session.resource.set.flag.request-id",
		"you.session.resource.set.flag.expected-revision",
		"you.session.resource.set.flag.reason",
	} {
		if err := handlers.ResourceSet(cmd, resolvedTestInputsWithout(t, base, omitted), inherited); err == nil {
			t.Fatalf("ResourceSet() with %s omitted = nil, want stable input error", omitted)
		}
	}
}

func TestNewRepresentativeRegistryRejectsMissingHandlers(t *testing.T) {
	if _, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{}); err == nil {
		t.Fatal("NewRepresentativeRegistry() missing root handler = nil, want error")
	}
}

func stringPtr(value string) *string {
	return &value
}

package commandregistry_test

import (
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

func TestNewRepresentativeRegistryRejectsMissingHandlers(t *testing.T) {
	if _, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{}); err == nil {
		t.Fatal("NewRepresentativeRegistry() missing root handler = nil, want error")
	}
}

func stringPtr(value string) *string {
	return &value
}

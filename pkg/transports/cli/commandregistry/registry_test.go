package commandregistry_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestRegistry_RegisterRejectsDuplicateCommandID(t *testing.T) {
	registry := commandregistry.NewRegistry()
	handler := func(cmd *cobra.Command, args []string) error { return nil }
	if err := registry.Register("you.session.show", handler); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.Register("you.session.show", handler); err == nil {
		t.Fatal("Register() duplicate = nil, want error")
	}
}

func TestRegistry_LookupRejectsMissingCommandID(t *testing.T) {
	registry := commandregistry.NewRegistry()
	if _, err := registry.Lookup("you.session.show"); err == nil {
		t.Fatal("Lookup() missing = nil, want error")
	}
}

func TestRegistry_AttachRunESetsHandwrittenHandler(t *testing.T) {
	registry := commandregistry.NewRegistry()
	wantErr := errors.New("handwritten handler invoked")
	if err := registry.Register("you.session.show", func(cmd *cobra.Command, args []string) error {
		return wantErr
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	cmd := &cobra.Command{Use: "show"}
	if err := registry.AttachRunE(cmd, "you.session.show"); err != nil {
		t.Fatalf("AttachRunE() error = %v", err)
	}
	if cmd.RunE == nil {
		t.Fatal("AttachRunE() left RunE nil")
	}
	if err := cmd.RunE(cmd, nil); !errors.Is(err, wantErr) {
		t.Fatalf("RunE() error = %v, want %v", err, wantErr)
	}
}

func TestRegistry_RejectsNilRegistryOperations(t *testing.T) {
	var registry *commandregistry.Registry
	if err := registry.Register("you", noopRegistryRunE); err == nil {
		t.Fatal("Register() on nil registry = nil, want error")
	}
	if _, err := registry.Lookup("you"); err == nil {
		t.Fatal("Lookup() on nil registry = nil, want error")
	}
	if err := registry.AttachRunE(&cobra.Command{}, "you"); err == nil {
		t.Fatal("AttachRunE() on nil registry = nil, want error")
	}
}

func TestRegistry_RegisterRejectsInvalidInput(t *testing.T) {
	registry := commandregistry.NewRegistry()
	handler := noopRegistryRunE
	if err := registry.Register("", handler); err == nil {
		t.Fatal("Register() empty command ID = nil, want error")
	}
	if err := registry.Register("you.session.show", nil); err == nil {
		t.Fatal("Register() nil handler = nil, want error")
	}
}

func TestRegistry_AttachRunERejectsNilCommand(t *testing.T) {
	registry := commandregistry.NewRegistry()
	if err := registry.AttachRunE(nil, "you.session.show"); err == nil {
		t.Fatal("AttachRunE() nil command = nil, want error")
	}
}

func noopRegistryRunE(cmd *cobra.Command, args []string) error { return nil }

func noopSubmitHandler(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}

func TestNewSubmitRegistryRequiresCompleteStableHandlerCoverage(t *testing.T) {
	tests := []struct {
		name     string
		handlers commandregistry.SubmitHandlers
		wantID   string
	}{
		{
			name:     "missing unary",
			handlers: commandregistry.SubmitHandlers{SubmitBatch: noopSubmitHandler},
			wantID:   commandregistry.SubmitHandlerID,
		},
		{
			name:     "missing batch",
			handlers: commandregistry.SubmitHandlers{Submit: noopSubmitHandler},
			wantID:   commandregistry.SubmitBatchHandlerID,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := commandregistry.NewSubmitRegistry(test.handlers)
			if err == nil || !strings.Contains(err.Error(), test.wantID) {
				t.Fatalf("NewSubmitRegistry() error = %v, want stable handler ID %q", err, test.wantID)
			}
		})
	}
}

func TestSubmitRegistryVerifiesOnlyCanonicalHandlerIDs(t *testing.T) {
	registry := mustSubmitRegistry(t)
	manifest := submitRegistryManifest(t)
	if err := registry.Verify(manifest); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	for _, handlerID := range []string{
		commandregistry.SubmitHandlerID,
		commandregistry.SubmitBatchHandlerID,
	} {
		if _, err := registry.Lookup(handlerID); err != nil {
			t.Fatalf("Lookup(%q) error = %v", handlerID, err)
		}
	}
	if _, err := registry.Lookup("you.submit.unknown.handler"); err == nil {
		t.Fatal("Lookup(unknown) error = nil, want rejection")
	}
}

func TestSubmitRegistryRejectsInvalidManifestHandlers(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(climanifest.Manifest)
	}{
		{
			name: "unknown",
			mutate: func(manifest climanifest.Manifest) {
				record := manifest.Commands["you.submit"]
				record.Handler.ID = "you.submit.unknown.handler"
				manifest.Commands[record.ID] = record
			},
		},
		{
			name: "missing",
			mutate: func(manifest climanifest.Manifest) {
				record := manifest.Commands["you.submit"]
				record.Handler = nil
				manifest.Commands[record.ID] = record
			},
		},
		{
			name: "duplicate",
			mutate: func(manifest climanifest.Manifest) {
				record := manifest.Commands["you.submit.batch"]
				record.Handler.ID = commandregistry.SubmitHandlerID
				manifest.Commands[record.ID] = record
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := submitRegistryManifest(t)
			test.mutate(manifest)
			if err := mustSubmitRegistry(t).Verify(manifest); err == nil {
				t.Fatal("Verify() error = nil, want rejection")
			}
		})
	}
}

func mustSubmitRegistry(t *testing.T) *commandregistry.SubmitRegistry {
	t.Helper()
	registry, err := commandregistry.NewSubmitRegistry(commandregistry.SubmitHandlers{
		Submit:      noopSubmitHandler,
		SubmitBatch: noopSubmitHandler,
	})
	if err != nil {
		t.Fatalf("NewSubmitRegistry() error = %v", err)
	}
	return registry
}

func submitRegistryManifest(t *testing.T) climanifest.Manifest {
	t.Helper()
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		t.Fatalf("RunSubmitFamilyManifest() error = %v", err)
	}
	return climanifest.Manifest{
		FormatVersion: manifest.FormatVersion,
		RootPath:      manifest.RootPath,
		Commands: map[string]climanifest.Command{
			"you.submit":       manifest.Commands["you.submit"],
			"you.submit.batch": manifest.Commands["you.submit.batch"],
		},
	}
}

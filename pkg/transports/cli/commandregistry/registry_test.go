package commandregistry_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
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

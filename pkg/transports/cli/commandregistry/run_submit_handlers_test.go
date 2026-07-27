package commandregistry_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

func TestNewRunServerRegistryAttachesCompleteHandwrittenLifecycles(t *testing.T) {
	preRuns := 0
	preRun := func(*cobra.Command, []string) error {
		preRuns++
		return nil
	}
	registry, err := commandregistry.NewRunServerRegistry(commandregistry.RunServerHandlers{
		Run:    commandregistry.CommandHandlers{PreRunE: preRun, RunE: noopRunE},
		Server: commandregistry.CommandHandlers{PreRunE: preRun, RunE: noopRunE},
	})
	if err != nil {
		t.Fatalf("NewRunServerRegistry() error = %v", err)
	}
	for _, commandID := range []string{"you.run", "you.server"} {
		cmd := &cobra.Command{Use: commandID}
		if err := registry.AttachHandlers(cmd, commandID); err != nil {
			t.Fatalf("AttachHandlers(%s) error = %v", commandID, err)
		}
		if cmd.PreRunE == nil || cmd.RunE == nil {
			t.Fatalf("%s lifecycle incomplete", commandID)
		}
		if err := cmd.PreRunE(cmd, nil); err != nil {
			t.Fatalf("%s PreRunE error = %v", commandID, err)
		}
	}
	if preRuns != 2 {
		t.Fatalf("PreRunE calls = %d, want 2", preRuns)
	}
}

func TestNewRunServerRegistryRejectsMissingHandler(t *testing.T) {
	_, err := commandregistry.NewRunServerRegistry(commandregistry.RunServerHandlers{
		Run:    commandregistry.CommandHandlers{PreRunE: noopRunE, RunE: noopRunE},
		Server: commandregistry.CommandHandlers{},
	})
	if err == nil {
		t.Fatal("NewRunServerRegistry() missing server handler = nil, want error")
	}
}

func TestNewRunServerRegistryRejectsMissingPreRunHandler(t *testing.T) {
	_, err := commandregistry.NewRunServerRegistry(commandregistry.RunServerHandlers{
		Run:    commandregistry.CommandHandlers{RunE: noopRunE},
		Server: commandregistry.CommandHandlers{PreRunE: noopRunE, RunE: noopRunE},
	})
	if err == nil {
		t.Fatal("NewRunServerRegistry() missing run PreRunE = nil, want error")
	}
}

func TestVerifyRunServerRunnableCoverageRejectsOutOfFamilyHandler(t *testing.T) {
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		t.Fatalf("RunSubmitFamilyManifest() error = %v", err)
	}
	registry, err := commandregistry.NewRunServerRegistry(commandregistry.RunServerHandlers{
		Run:    commandregistry.CommandHandlers{PreRunE: noopRunE, RunE: noopRunE},
		Server: commandregistry.CommandHandlers{PreRunE: noopRunE, RunE: noopRunE},
	})
	if err != nil {
		t.Fatalf("NewRunServerRegistry() error = %v", err)
	}
	if err := registry.Register("you.work.list", noopRunE); err != nil {
		t.Fatalf("Register(out-of-family) error = %v", err)
	}
	if err := registry.VerifyRunServerRunnableCoverage(manifest); err == nil {
		t.Fatal("out-of-family handler binding = nil, want error")
	}
}

func TestVerifyRunServerRunnableCoverageRejectsMissingHandler(t *testing.T) {
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		t.Fatalf("RunSubmitFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you.run", noopRunE); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.VerifyRunServerRunnableCoverage(manifest); err == nil {
		t.Fatal("VerifyRunServerRunnableCoverage() missing server handler = nil, want error")
	}
}

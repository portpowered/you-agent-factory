package commandregistry_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	cligenerated "github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

func TestNewWorkRegistryRegistersContractedRunnableIDs(t *testing.T) {
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE: noopRunE, ShowRunE: noopRunE, MoveRunE: noopRunE, VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}
	for _, commandID := range []string{
		"you.work.list", "you.work.show", "you.work.move", "you.work.visualize",
	} {
		if _, err := registry.Lookup(commandID); err != nil {
			t.Fatalf("Lookup(%q) error = %v", commandID, err)
		}
	}
}

func TestNewWorkRegistryRejectsMissingHandlers(t *testing.T) {
	cases := []struct {
		name     string
		handlers commandregistry.WorkHandlers
	}{
		{
			name: "list",
			handlers: commandregistry.WorkHandlers{
				ShowRunE: noopRunE, MoveRunE: noopRunE, VisualizeRunE: noopRunE,
			},
		},
		{
			name: "show",
			handlers: commandregistry.WorkHandlers{
				ListRunE: noopRunE, MoveRunE: noopRunE, VisualizeRunE: noopRunE,
			},
		},
		{
			name: "move",
			handlers: commandregistry.WorkHandlers{
				ListRunE: noopRunE, ShowRunE: noopRunE, VisualizeRunE: noopRunE,
			},
		},
		{
			name: "visualize",
			handlers: commandregistry.WorkHandlers{
				ListRunE: noopRunE, ShowRunE: noopRunE, MoveRunE: noopRunE,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := commandregistry.NewWorkRegistry(tc.handlers); err == nil {
				t.Fatal("NewWorkRegistry() missing handler = nil, want error")
			}
		})
	}
}

func TestRunnableWorkCommandIDsFromGeneratedManifest(t *testing.T) {
	manifest, err := cligenerated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	ids, err := commandregistry.RunnableWorkCommandIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableWorkCommandIDs() error = %v", err)
	}
	want := []string{"you.work.list", "you.work.move", "you.work.show", "you.work.visualize"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("runnable IDs = %v, want %v", ids, want)
	}
}

func TestVerifyWorkRunnableCoverageRejectsMissingHandler(t *testing.T) {
	manifest, err := cligenerated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	for _, commandID := range []string{"you.work.show", "you.work.move", "you.work.visualize"} {
		if err := registry.Register(commandID, noopRunE); err != nil {
			t.Fatalf("Register(%q) error = %v", commandID, err)
		}
	}
	if err := registry.VerifyWorkRunnableCoverage(manifest); err == nil {
		t.Fatal("VerifyWorkRunnableCoverage() missing list handler = nil, want error")
	}
}

func TestVerifyWorkRunnableCoverageAcceptsCompleteRegistry(t *testing.T) {
	manifest, err := cligenerated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	for _, commandID := range []string{
		"you.work.list", "you.work.show", "you.work.move", "you.work.visualize",
	} {
		if err := registry.Register(commandID, noopRunE); err != nil {
			t.Fatalf("Register(%q) error = %v", commandID, err)
		}
	}
	if err := registry.VerifyWorkRunnableCoverage(manifest); err != nil {
		t.Fatalf("VerifyWorkRunnableCoverage() error = %v", err)
	}
}

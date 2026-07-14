package commandregistry_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

func TestRunnableWorkCommandIDsFromGeneratedManifest(t *testing.T) {
	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	ids, err := commandregistry.RunnableWorkCommandIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableWorkCommandIDs() error = %v", err)
	}
	want := []string{
		"you.work.list",
		"you.work.move",
		"you.work.show",
		"you.work.visualize",
	}
	if len(ids) != len(want) {
		t.Fatalf("runnable IDs = %#v, want %#v", ids, want)
	}
	for i, commandID := range want {
		if ids[i] != commandID {
			t.Fatalf("runnable IDs[%d] = %q, want %q", i, ids[i], commandID)
		}
	}
}

func TestVerifyWorkRunnableCoverageRejectsMissingHandler(t *testing.T) {
	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	for _, commandID := range []string{
		"you.work.show",
		"you.work.move",
		"you.work.visualize",
	} {
		if err := registry.Register(commandID, noopRunE); err != nil {
			t.Fatalf("Register(%q) error = %v", commandID, err)
		}
	}
	if err := registry.VerifyWorkRunnableCoverage(manifest); err == nil {
		t.Fatal("VerifyWorkRunnableCoverage() missing list handler = nil, want error")
	}
}

func TestVerifyWorkRunnableCoverageAcceptsCompleteRegistry(t *testing.T) {
	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	for _, commandID := range []string{
		"you.work.list",
		"you.work.show",
		"you.work.move",
		"you.work.visualize",
	} {
		if err := registry.Register(commandID, noopRunE); err != nil {
			t.Fatalf("Register(%q) error = %v", commandID, err)
		}
	}
	if err := registry.VerifyWorkRunnableCoverage(manifest); err != nil {
		t.Fatalf("VerifyWorkRunnableCoverage() error = %v", err)
	}
}

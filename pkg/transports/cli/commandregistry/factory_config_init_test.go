package commandregistry_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

func TestRunnableFactoryConfigInitCommandIDsFromGeneratedManifest(t *testing.T) {
	manifest, err := generated.FactoryConfigInitFamilyManifest()
	if err != nil {
		t.Fatalf("FactoryConfigInitFamilyManifest() error = %v", err)
	}
	ids, err := commandregistry.RunnableFactoryConfigInitCommandIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableFactoryConfigInitCommandIDs() error = %v", err)
	}
	want := []string{
		"you.config.init",
		"you.factory.config.expand",
		"you.factory.config.flatten",
		"you.factory.config.validate",
		"you.factory.create",
		"you.factory.delete",
		"you.factory.list",
		"you.factory.query",
		"you.factory.replace-current",
		"you.factory.update",
		"you.init",
	}
	if len(ids) != len(want) {
		t.Fatalf("runnable IDs = %#v, want %#v", ids, want)
	}
	for i, id := range want {
		if ids[i] != id {
			t.Fatalf("runnable IDs[%d] = %q, want %q", i, ids[i], id)
		}
	}
}

func TestVerifyFactoryConfigInitRunnableCoverageRejectsMissingHandler(t *testing.T) {
	manifest, err := generated.FactoryConfigInitFamilyManifest()
	if err != nil {
		t.Fatalf("FactoryConfigInitFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you.factory.query", noopRunE); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.VerifyFactoryConfigInitRunnableCoverage(manifest); err == nil {
		t.Fatal("VerifyFactoryConfigInitRunnableCoverage() missing handlers = nil, want error")
	}
}

func TestVerifyFactoryConfigInitRunnableCoverageAcceptsCompleteRegistry(t *testing.T) {
	manifest, err := generated.FactoryConfigInitFamilyManifest()
	if err != nil {
		t.Fatalf("FactoryConfigInitFamilyManifest() error = %v", err)
	}
	runnableIDs, err := commandregistry.RunnableFactoryConfigInitCommandIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableFactoryConfigInitCommandIDs() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	for _, commandID := range runnableIDs {
		if err := registry.Register(commandID, noopRunE); err != nil {
			t.Fatalf("Register(%q) error = %v", commandID, err)
		}
	}
	if err := registry.VerifyFactoryConfigInitRunnableCoverage(manifest); err != nil {
		t.Fatalf("VerifyFactoryConfigInitRunnableCoverage() error = %v", err)
	}
}

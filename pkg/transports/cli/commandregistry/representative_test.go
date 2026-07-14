package commandregistry_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

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

func TestVerifyRepresentativeRunnableCoverageRejectsMissingHandler(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you.session.show", noopRunE); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.VerifyRepresentativeRunnableCoverage(manifest); err == nil {
		t.Fatal("VerifyRepresentativeRunnableCoverage() missing you handler = nil, want error")
	}
}

func TestVerifyRepresentativeRunnableCoverageAcceptsCompleteRegistry(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	for _, commandID := range []string{"you", "you.session.show"} {
		if err := registry.Register(commandID, noopRunE); err != nil {
			t.Fatalf("Register(%q) error = %v", commandID, err)
		}
	}
	if err := registry.VerifyRepresentativeRunnableCoverage(manifest); err != nil {
		t.Fatalf("VerifyRepresentativeRunnableCoverage() error = %v", err)
	}
}

func noopRunE(cmd *cobra.Command, args []string) error {
	return nil
}

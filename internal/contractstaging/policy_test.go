package contractstaging_test

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
)

func TestAllowedArtifactsAreTheReviewedJoinedContracts(t *testing.T) {
	want := []string{
		"packages/api/generated/cli/commands.json",
		"packages/api/generated/javascript/runtime-api.json",
		"packages/api/generated/joined/contracts/common/deprecations.schema.json",
		"packages/api/generated/joined/contracts/common/documentation.schema.json",
		"packages/api/generated/joined/contracts/manifest.schema.json",
		"packages/api/generated/manifest.json",
		"packages/api/generated/mcp/tools.json",
		"packages/api/generated/openapi/openapi.yaml",
		"packages/api/generated/schemas/factory-event.schema.json",
		"packages/api/generated/schemas/factory-recording.schema.json",
		"packages/api/generated/schemas/factory.schema.json",
		"packages/api/generated/schemas/mock-workers.schema.json",
		"packages/api/generated/schemas/you-config.schema.json",
	}
	if got := contractstaging.AllowedArtifacts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("AllowedArtifacts() = %q, want %q", got, want)
	}
}

func TestRawArtifactsReturnsIndependentPolicyCopies(t *testing.T) {
	first := contractstaging.RawArtifacts()
	first[0].Source = "changed"

	second := contractstaging.RawArtifacts()
	if second[0].Source == "changed" {
		t.Fatal("RawArtifacts() shared mutable policy state")
	}
}

func TestJoinInputReturnsIndependentPolicyCopies(t *testing.T) {
	first := contractstaging.JoinInput("first")
	first.Roots[0] = "changed"
	first.Components[0] = "changed"

	second := contractstaging.JoinInput("second")
	if second.RepositoryRoot != "second" || second.Roots[0] == "changed" || second.Components[0] == "changed" {
		t.Fatalf("JoinInput() shared mutable policy state: %#v", second)
	}
}

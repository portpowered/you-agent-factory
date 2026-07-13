package contractstaging_test

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
)

func TestAllowedArtifactsAreTheReviewedJoinedContracts(t *testing.T) {
	want := []string{
		"packages/api/generated/joined/contracts/common/deprecations.schema.json",
		"packages/api/generated/joined/contracts/common/documentation.schema.json",
		"packages/api/generated/joined/contracts/manifest.schema.json",
	}
	if got := contractstaging.AllowedArtifacts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("AllowedArtifacts() = %q, want %q", got, want)
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

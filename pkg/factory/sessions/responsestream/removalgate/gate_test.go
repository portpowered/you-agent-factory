package removalgate_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream/removalgate"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestPrivateContractRemovalGate_ConsolidatedEvidence(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	if err := removalgate.AssertGate(context.Background(), repoRoot); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateContractRemovalGate_DocsPrerequisite(t *testing.T) {
	if err := removalgate.AssertDocsPrerequisite(); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateContractRemovalGate_NoPrivateNDJSONInProductionSurfaces(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	if err := removalgate.AssertNoPrivateNDJSONInProductionSurfaces(repoRoot); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateContractRemovalGate_PublicTransportDoesNotImportLegacyCompat(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	if err := removalgate.AssertPublicTransportLayersDoNotImportLegacyCompat(repoRoot); err != nil {
		t.Fatal(err)
	}
}

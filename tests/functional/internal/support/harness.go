package support

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
)

// NewGuardsBatchHarness builds a service-mode harness for guards_batch tests
// that submit work after RunInBackground. Batch mode can terminate before the
// first post-run submit on slower CI runners.
func NewGuardsBatchHarness(t *testing.T, dir string, opts ...testutil.ServiceTestHarnessOption) *testutil.ServiceTestHarness {
	t.Helper()

	all := append([]testutil.ServiceTestHarnessOption{
		testutil.WithRuntimeMode(interfaces.RuntimeModeService),
	}, opts...)
	return testutil.NewServiceTestHarness(t, dir, all...)
}

func WaitForHarnessRuntimeAvailability(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	runErrCh <-chan error,
	timeout time.Duration,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := h.WaitForRuntimeAvailability(ctx, runErrCh); err != nil {
		t.Fatal(err)
	}
}

func RunGuardsBatchHarness(t *testing.T, h *testutil.ServiceTestHarness, ctx context.Context) <-chan error {
	t.Helper()

	errCh := h.RunInBackground(ctx)
	WaitForHarnessRuntimeAvailability(t, h, errCh, 15*time.Second)
	return errCh
}

func WaitForHarnessPlaceTokenCount(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	placeID string,
	want int,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot, err := h.GetEngineStateSnapshot()
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if PlaceTokenCount(snapshot.Marking, placeID) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	t.Fatalf("timed out waiting for %d token(s) in %s; marking=%#v", want, placeID, snapshot.Marking.PlaceTokens)
}

func PlaceTokenCount(marking petri.MarkingSnapshot, placeID string) int {
	return len(marking.PlaceTokens[placeID])
}

type TokenIdentitySet struct {
	WorkIDs    []string
	WorkTypes  []string
	TokenNames []string
}

func DeriveTokenIdentities(
	consumedTokens []factorytoken.Token,
	outputMutations []interfaces.TokenMutationRecord,
) TokenIdentitySet {
	var identities TokenIdentitySet

	for _, token := range consumedTokens {
		addWorkTokenIdentity(&identities, token)
	}
	for _, mutation := range outputMutations {
		if mutation.Token == nil {
			continue
		}
		addWorkTokenIdentity(&identities, *mutation.Token)
	}
	return identities
}

func addWorkTokenIdentity(identities *TokenIdentitySet, token factorytoken.Token) {
	if token.Color.DataType == factorytoken.DataTypeResource {
		return
	}
	if token.Color.WorkID != "" {
		identities.WorkIDs = appendDistinct(identities.WorkIDs, token.Color.WorkID)
	}
	if token.Color.WorkTypeID != "" {
		identities.WorkTypes = appendDistinct(identities.WorkTypes, token.Color.WorkTypeID)
	}
	if token.Color.Name != "" {
		identities.TokenNames = appendDistinct(identities.TokenNames, token.Color.Name)
	}
}

func appendDistinct(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

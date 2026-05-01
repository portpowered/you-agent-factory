package support

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/testutil"
)

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

func HasWorkTokenInPlace(marking petri.MarkingSnapshot, placeID, workID string) bool {
	for _, tok := range marking.Tokens {
		if tok.PlaceID == placeID && tok.Color.WorkID == workID {
			return true
		}
	}
	return false
}

func CountFactoryEvents(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

type TokenIdentitySet struct {
	WorkIDs    []string
	WorkTypes  []string
	TokenNames []string
}

func DeriveTokenIdentities(
	consumedTokens []interfaces.Token,
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

func addWorkTokenIdentity(identities *TokenIdentitySet, token interfaces.Token) {
	if token.Color.DataType == interfaces.DataTypeResource {
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

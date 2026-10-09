package service

import (
	"context"
	"errors"
	"fmt"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"path/filepath"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestCurrentBoardQuarantineDispositionRequiresSafeCauseAndPreservation(t *testing.T) {
	t.Parallel()
	for _, cause := range []string{"INVALID_JSON", "INVALID_SCHEMA", "SIZE_LIMIT", "READ_FAILED", "UNKNOWN", "unclassified", "preservation failure", "canceled preservation"} {
		t.Run(cause, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			owner := &quarantineOpeningOwner{}
			opening := &sessionRuntimeOpening{clock: openingCoordinatorClock{}, durableExecution: DurableExecution{Service: owner}, sessionSelection: &factorysessions.SessionRuntimeSelection{}}
			opening.configured.Recordings.RecordPath = "retained-history"
			opening.sessionSelection.Recording.RecordPath = "retained-history"
			var failure error = classifiedBoardFailure(cause)
			wantCalls, wantSuccess := 1, true
			switch cause {
			case "UNKNOWN":
				wantCalls, wantSuccess = 0, false
			case "unclassified":
				failure = errors.New("unclassified probe failure")
				wantCalls, wantSuccess = 0, false
			case "preservation failure":
				failure = classifiedBoardFailure("INVALID_JSON")
				owner.failure = errors.New("preservation failed")
				wantSuccess = false
			case "canceled preservation":
				failure = classifiedBoardFailure("INVALID_JSON")
				owner.onPreserve = cancel
				wantSuccess = false
			}
			root := &RuntimeOpening{generateSessionID: func() string { return "identity" }}
			err := root.quarantineUnreadableCurrentBoard(ctx, opening, fmt.Errorf("probe: %w", failure))
			if (err == nil) != wantSuccess || owner.calls != wantCalls {
				t.Fatalf("error=%v, preservation calls=%d", err, owner.calls)
			}
			assertCurrentBoardQuarantineDisposition(t, opening, cause, wantSuccess)
		})
	}
}

func assertCurrentBoardQuarantineDisposition(t *testing.T, opening *sessionRuntimeOpening, cause string, wantSuccess bool) {
	t.Helper()
	if wantSuccess {
		if !opening.emptyCurrentBoard || opening.startupRecovery == nil || opening.startupRecovery.cause != cause || opening.configured.Recordings.RecordPath != "" || opening.sessionSelection.Recording.RecordPath != "" {
			t.Fatal("successful preservation did not select diagnosed fresh board")
		}
	} else if opening.emptyCurrentBoard || opening.startupRecovery != nil || opening.configured.Recordings.RecordPath != "retained-history" {
		t.Fatal("failed preservation published fresh-board success")
	}
}

func TestRuntimeOpeningLegacyInventoryUsesSelectedProfileAndPreservesFailure(t *testing.T) {
	t.Parallel()
	cause := errors.New("selected inventory unavailable")
	inventory := &openingInventoryFailure{cause: cause}
	owner := &RuntimeOpening{recordedInventory: inventory}
	opening := &sessionRuntimeOpening{
		sessionID:        "selected-session",
		sessionSelection: &factorysessions.SessionRuntimeSelection{SystemConfigHome: preparationPath("selected-profile")},
		durableExecution: DurableExecution{Service: &openingBoardFacts{}},
	}
	_, err := owner.discoverLegacyCurrentBoard(t.Context(), opening)
	if !errors.Is(err, cause) {
		t.Fatalf("inventory cause lost: %v", err)
	}
	want := filepath.Join(opening.sessionSelection.SystemConfigHome, ".you-agent-factory", "recordings")
	if inventory.request.RecordingRoot != want || inventory.calls != 1 {
		t.Fatalf("inventory selection = %+v, calls=%d", inventory.request, inventory.calls)
	}
}

type openingInventoryFailure struct {
	request recordings.RecordedSessionInventoryRequest
	cause   error
	calls   int
}

func (inventory *openingInventoryFailure) ListRecordedSessions(request recordings.RecordedSessionInventoryRequest) (recordings.RecordedSessionInventoryResult, error) {
	inventory.request = request
	inventory.calls++
	return recordings.RecordedSessionInventoryResult{}, inventory.cause
}

type openingBoardFacts struct{ quarantineOpeningOwner }

func (*openingBoardFacts) LoadCurrentBoardFacts(context.Context, string) ([]factorydefinitions.FactoryEvent, error) {
	return nil, nil
}
func (*openingBoardFacts) MatchCurrentBoardWork(context.Context, string, *factorydefinitions.FactoryWorldState) (bool, error) {
	return false, nil
}

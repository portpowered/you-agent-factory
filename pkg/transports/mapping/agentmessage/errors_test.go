package agentmessage

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"testing"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestMessageErrorMapsStableCodesWithoutCollaboratorDiagnostics(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{agentmessages.ErrBadRequest, 400, "MESSAGE_INVALID_REQUEST"},
		{agentmessages.ErrCursorInvalid, 400, "MESSAGE_CURSOR_INVALID"},
		{agentmessages.ErrInterruptUnsupported, 400, "MESSAGE_INTERRUPT_UNSUPPORTED"},
		{workersessions.ErrCallerInvalid, 403, "WORKER_SESSION_CALLER_INVALID"},
		{agentmessages.ErrNotPermitted, 403, "MESSAGE_NOT_PERMITTED"},
		{agentmessages.ErrRecipientNotFound, 404, "MESSAGE_RECIPIENT_NOT_FOUND"},
		{agentmessages.ErrMessageNotFound, 404, "MESSAGE_NOT_FOUND"},
		{workersessions.ErrWorkerSessionAmbiguous, 409, "WORKER_SESSION_AMBIGUOUS"},
		{agentmessages.ErrRequestConflict, 409, "MESSAGE_REQUEST_CONFLICT"},
		{agentmessages.ErrLimitExceeded, 429, "MESSAGE_LIMIT_EXCEEDED"},
		{agentmessages.ErrDisabled, 503, "MESSAGING_DISABLED"},
		{agentmessages.ErrStoreUnavailable, 503, "MESSAGE_STORE_UNAVAILABLE"},
		{agentmessages.ErrStoreCorrupt, 503, "MESSAGE_STORE_CORRUPT"},
		{agentmessages.ErrStreamUnavailable, 503, "MESSAGE_STREAM_UNAVAILABLE"},
		{agentmessages.ErrStreamGap, 410, "MESSAGE_STREAM_GAP"},
		{agentmessages.ErrStreamBackpressure, 503, "MESSAGE_STREAM_BACKPRESSURE"},
		{errors.New("planted-secret token"), 500, "INTERNAL_ERROR"},
	} {
		t.Run(tt.code, func(t *testing.T) {
			status, response := (Mapper{}).Error(fmt.Errorf("planted-secret token: %w", tt.err))
			encoded, err := json.Marshal(response)
			if err != nil || status != tt.status || string(response.Code) != tt.code || response.Message != tt.code || strings.Contains(string(encoded), "planted-secret") {
				t.Fatalf("unsafe or incorrect error: %d %s %v", status, encoded, err)
			}
		})
	}
}

func TestMessageErrorPreservesDetachedAmbiguityAndLimitDetails(t *testing.T) {
	t.Parallel()
	work := "work"
	ambiguous := &workersessions.AmbiguousAddressError{Candidates: []workersessions.AddressCandidate{
		{FactorySessionID: "b", WorkerSessionID: "legacy", WorkID: &work, State: workersessions.StateRunning},
		{FactorySessionID: "a", WorkerSessionID: "legacy", State: workersessions.StateCompleted},
	}}
	status, response := (Mapper{}).Error(ambiguous)
	work = "changed"
	details := response.Details.(factoryapi.WorkerSessionAddressDetails)
	if status != http.StatusConflict || response.Family != factoryapi.ErrorFamilyConflict || len(details.Candidates) != 2 || details.Candidates[0].FactorySessionId != "a" || *details.Candidates[1].WorkId != "work" {
		t.Fatal("ambiguity lost exact identities, order or detachment")
	}
	status, response = (Mapper{}).Error(&agentmessages.LimitError{Dimension: "sender", RetryAfterSeconds: 12})
	encoded, err := json.Marshal(response)
	if err != nil || status != 429 || !strings.Contains(string(encoded), `"dimension":"sender","retryAfterSeconds":12`) {
		t.Fatalf("lost rate-limit details: %s %v", encoded, err)
	}
}

func TestObservationMappingPreservesEnvelopeAndRejectsIntegerOverflow(t *testing.T) {
	t.Parallel()
	input := agentmessages.Observation{RecordID: "transaction", Sequence: 42, Kind: "SENT", Message: agentmessages.Message{MessageID: "m", Body: "[REDACTED]"}}
	output, err := (Mapper{}).Observation(input)
	if err != nil || output.RecordId != input.RecordID || output.Sequence != 42 || string(output.Kind) != input.Kind || output.Message.Body != input.Message.Body {
		t.Fatal("observation mapping changed admitted facts", err)
	}
	input.Sequence = math.MaxUint64
	if _, err := (Mapper{}).Observation(input); !errors.Is(err, agentmessages.ErrStreamUnavailable) {
		t.Fatal("sequence overflow was silently published", err)
	}
}

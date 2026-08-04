package mapping

import (
	"encoding/json"
	"fmt"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ProjectSessionInfoUpdate projects one SESSION-kind, workers.PhaseUpdated
// record -- the source representation for a mid-lifecycle Chat metadata
// change, distinct from KindSession's lifecycle phases (STARTED/COMPLETED/
// FAILED/CANCELED, which stay NO_OUTPUT and never reach this function; see
// Project's dispatch) -- into one session_info_update, or reports that this
// record carries no ACP-supported metadata change to report.
//
// workers.SessionPayload.Title is the one ACP-supported customer metadata
// field this L1 slice currently projects, mirroring
// acpsdk.SessionSessionInfoUpdate.Title's own nullable "set to null to
// clear" semantics: a nil Title means the record declared no title change,
// so this reports (nil, nil) rather than an empty update, the same
// "meaningful data required" convention ProjectUsage applies to a
// zero-token UsagePayload. UpdatedAt is left unset -- no committed record
// timestamp is threaded through workers.Draft to this pure projector, the
// same reason ProjectUsage leaves acpsdk.SessionUsageUpdate.Size zero rather
// than fabricating it. A malformed payload reports (nil, ErrMalformedRecord).
func ProjectSessionInfoUpdate(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	if draft.Phase != workers.PhaseUpdated {
		return nil, nil
	}

	var payload workers.SessionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		return nil, fmt.Errorf("%w: payload must decode as SessionPayload: %v", ErrMalformedRecord, err)
	}

	if payload.Title == nil {
		return nil, nil
	}

	return &acpsdk.SessionUpdate{
		SessionInfoUpdate: &acpsdk.SessionSessionInfoUpdate{Title: payload.Title},
	}, nil
}

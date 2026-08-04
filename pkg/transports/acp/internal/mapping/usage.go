package mapping

import (
	"encoding/json"
	"fmt"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ProjectUsage projects one USAGE-kind record -- workers.PhaseUpdated, the
// only phase response-draft validation allows for this Kind -- into one
// usage_update, or reports that this record carries no meaningful usage to
// report. "Meaningful" means at least one primary token count is present;
// a record that carries only workers.UsagePayload.Model, or nothing at all,
// produces (nil, nil) rather than an update with a zero token count.
//
// acpsdk.SessionUsageUpdate.Size ("total context window size") has no
// source in workers.UsagePayload -- this L1 slice never learns a provider's
// context-window capacity, only its consumption -- so Size is left at its
// zero value rather than being fabricated from Used. A malformed payload
// reports (nil, ErrMalformedRecord).
func ProjectUsage(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	var payload workers.UsagePayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		return nil, fmt.Errorf("%w: payload must decode as UsagePayload: %v", ErrMalformedRecord, err)
	}

	used := payload.TotalTokens
	if used <= 0 {
		used = payload.InputTokens + payload.OutputTokens
	}
	if used <= 0 {
		return nil, nil
	}

	return &acpsdk.SessionUpdate{
		UsageUpdate: &acpsdk.SessionUsageUpdate{Used: int(used)},
	}, nil
}

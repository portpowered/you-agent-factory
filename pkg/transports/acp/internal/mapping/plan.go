package mapping

import (
	"fmt"
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// defaultPlanEntryPriority is the priority reported for a plan entry.
//
// ACP requires a priority on every plan entry and the field carries no
// omitempty, so leaving it unset would serialize an empty string -- an invalid
// enum value on the wire. Our own PlanStep has no priority to map from. This is
// therefore a declared protocol-completion policy rather than an inferred
// value: medium is the neutral choice, and it is a named constant so a reader
// can see it is supplied by us and not by the Factory.
const defaultPlanEntryPriority = acpsdk.PlanEntryPriorityMedium

// ProjectPlan projects a session-level plan record into ACP's plan update.
//
// A plan owned by one Worker Session stays a tool_call_update instead (see
// projectChildPlan): promoting a worker's plan to the session level would let
// two concurrent workers overwrite each other's plan, since ACP models the
// session plan as a single replaceable list.
func ProjectPlan(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	var payload workers.PlanPayload
	if err := decodeChildPayload(draft.Payload, &payload, "PlanPayload"); err != nil {
		return nil, err
	}
	if len(payload.Steps) == 0 {
		// A plan with no steps carries nothing a client can render as a plan.
		// Reporting an empty plan would clear whatever the client is showing.
		return nil, nil
	}

	entries := make([]acpsdk.PlanEntry, 0, len(payload.Steps))
	for _, step := range payload.Steps {
		if strings.TrimSpace(step.Description) == "" {
			return nil, fmt.Errorf("%w: PlanPayload step description is required", ErrMalformedRecord)
		}
		status, err := planEntryStatus(step.Status)
		if err != nil {
			return nil, err
		}
		entries = append(entries, acpsdk.PlanEntry{
			Content:  step.Description,
			Status:   status,
			Priority: defaultPlanEntryPriority,
		})
	}
	return &acpsdk.SessionUpdate{Plan: &acpsdk.SessionUpdatePlan{Entries: entries}}, nil
}

// planEntryStatus maps our free-form step status onto ACP's closed enum.
//
// It fails closed on an unrecognized value rather than defaulting, matching how
// child lifecycle status is already handled: silently coercing an unknown
// status would report a plan state the Factory never claimed.
func planEntryStatus(status string) (acpsdk.PlanEntryStatus, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "pending":
		return acpsdk.PlanEntryStatusPending, nil
	case "in_progress", "running", "active":
		return acpsdk.PlanEntryStatusInProgress, nil
	case "completed", "complete", "done":
		return acpsdk.PlanEntryStatusCompleted, nil
	}
	return "", fmt.Errorf("%w: PlanPayload step status %q is not projectable", ErrMalformedRecord, status)
}

package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// WorkReadRoot is the narrow Recordings projection port needed to answer Work
// list/get reads. It deliberately excludes lifecycle and scope operations so
// this construction path and its tests do not depend on unrelated Recordings
// capabilities.
type WorkReadRoot interface {
	SubscribeFrom(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error)
	ReconstructWorldState(recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error)
}

// WorkSnapshotReader reads one session-scoped, detached Work read snapshot.
// Work owns the consumer side of this shape and never names Recordings to
// obtain it; Recordings owns this implementation and offers it to composition.
type WorkSnapshotReader interface {
	ReadWorkSnapshot(context.Context, string) (work.ReadSnapshot, error)
}

// NewWorkSnapshotReader projects Recordings canonical facts into the detached
// Work read snapshot Work state access consumes when no live Factory Session
// is available. A nil root yields a nil reader so composition can select the
// live session path instead.
func NewWorkSnapshotReader(root WorkReadRoot) WorkSnapshotReader {
	if root == nil {
		return nil
	}
	return workSnapshotReader{root: root}
}

type workSnapshotReader struct {
	root WorkReadRoot
}

func (r workSnapshotReader) ReadWorkSnapshot(
	ctx context.Context,
	sessionID string,
) (work.ReadSnapshot, error) {
	if r.root == nil {
		return work.ReadSnapshot{}, errors.New("Recordings service is required")
	}
	if err := requireSnapshotContext(ctx); err != nil {
		return work.ReadSnapshot{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return work.ReadSnapshot{}, recordings.ErrInvalidProjectionScope
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: sessionID}
	events, err := r.canonicalEventsForScope(ctx, scope)
	if err != nil {
		return work.ReadSnapshot{}, err
	}
	reconstructed, err := r.root.ReconstructWorldState(
		reconstructWorldStateRequest(scope, events),
	)
	if err != nil {
		return work.ReadSnapshot{}, err
	}
	snapshot, err := readSnapshotFromWorldState(reconstructed.WorldState)
	if err != nil {
		return work.ReadSnapshot{}, err
	}
	annotateWorkStateSequences(&snapshot, events)
	snapshot.Admissions = workAdmissionsFromCanonicalEvents(events)
	return snapshot, nil
}

// annotateWorkStateSequences attaches the latest canonical event position
// known to have produced each projected Work state. The projection itself is
// still the source of the state value; this pass only preserves the cursor
// needed for the Work-owned durability comparison.
func annotateWorkStateSequences(snapshot *work.ReadSnapshot, events []recordings.CanonicalEvent) {
	if snapshot == nil {
		return
	}
	if snapshot.StreamGenerationID == "" {
		for _, event := range events {
			if generation := strings.TrimSpace(string(event.Cursor.StreamGenerationID)); generation != "" {
				snapshot.StreamGenerationID = generation
				break
			}
		}
	}
	positions := make(map[string]int64)
	known := make(map[string]bool)
	for _, event := range events {
		sequence := int64(event.Cursor.Sequence)
		if sequence == 0 {
			sequence = int64(event.Sequence)
		}
		for _, workID := range workIDsFromCanonicalEvent(event) {
			if !known[workID] || sequence > positions[workID] {
				positions[workID] = sequence
				known[workID] = true
			}
		}
	}
	for index := range snapshot.Items {
		workID := snapshot.Items[index].WorkID
		if !known[workID] {
			continue
		}
		snapshot.Items[index].CurrentStateSequence = positions[workID]
		snapshot.Items[index].CurrentStateSequenceKnown = true
	}
}

func workIDsFromCanonicalEvent(event recordings.CanonicalEvent) []string {
	ids := make([]string, 0)
	appendID := func(workID string) {
		if strings.TrimSpace(workID) == "" {
			return
		}
		for _, existing := range ids {
			if existing == workID {
				return
			}
		}
		ids = append(ids, workID)
	}
	switch event.Kind {
	case recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest):
		appendCanonicalContextWorkIDs(event, appendID)
		var payload work.WorkRequestEventPayload
		if json.Unmarshal([]byte(event.Payload), &payload) == nil {
			for _, item := range payload.Works {
				appendID(item.WorkID)
			}
		}
		var legacy factorydefinitions.WorkRequestPayload
		if json.Unmarshal([]byte(event.Payload), &legacy) == nil {
			for _, item := range legacy.WorkItems {
				appendID(item.ID)
			}
		}
	case recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkStateChange):
		appendCanonicalContextWorkIDs(event, appendID)
		var payload factorydefinitions.WorkStateChangeEventPayload
		if json.Unmarshal([]byte(event.Payload), &payload) == nil {
			appendID(payload.WorkID)
		}
	case recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeDispatchResponse):
		appendCanonicalContextWorkIDs(event, appendID)
		var payload factorydefinitions.WorkstationResponsePayload
		if json.Unmarshal([]byte(event.Payload), &payload) == nil {
			for _, item := range payload.OutputWork {
				appendID(item.ID)
			}
		}
	}
	return ids
}

func appendCanonicalContextWorkIDs(event recordings.CanonicalEvent, appendID func(string)) {
	var context factorydefinitions.FactoryEventContext
	if json.Unmarshal([]byte(event.SourceContext), &context) != nil || context.WorkIDs == nil {
		return
	}
	for _, workID := range *context.WorkIDs {
		appendID(workID)
	}
}

func workAdmissionsFromCanonicalEvents(events []recordings.CanonicalEvent) []work.WorkAdmission {
	admissions := make([]work.WorkAdmission, 0)
	for _, event := range events {
		if event.Kind != recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest) {
			continue
		}
		var payload work.WorkRequestEventPayload
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			continue
		}
		var context factorydefinitions.FactoryEventContext
		_ = json.Unmarshal([]byte(event.SourceContext), &context)
		for index, item := range payload.Works {
			workID := item.WorkID
			if workID == "" && context.WorkIDs != nil && index < len(*context.WorkIDs) {
				workID = (*context.WorkIDs)[index]
			}
			if workID == "" || item.Name == "" {
				continue
			}
			admissions = append(admissions, work.WorkAdmission{
				WorkID: workID,
				Name:   item.Name,
				Order:  len(admissions),
			})
		}
	}
	return admissions
}

func (r workSnapshotReader) canonicalEventsForScope(
	ctx context.Context,
	scope recordings.CanonicalEventScope,
) ([]recordings.CanonicalEvent, error) {
	subscribed, err := r.root.SubscribeFrom(ctx, recordings.SubscribeRequest{Scope: scope})
	if err != nil {
		return nil, fmt.Errorf("subscribe Recordings canonical facts: %w", err)
	}
	return canonicalEventsFromSubscription(ctx, subscribed.Subscription)
}

func reconstructWorldStateRequest(
	scope recordings.CanonicalEventScope,
	events []recordings.CanonicalEvent,
) recordings.ReconstructWorldStateRequest {
	selectedTick := 0
	for _, event := range events {
		if tick := event.FactoryTick; tick > selectedTick {
			selectedTick = tick
		}
	}
	return recordings.ReconstructWorldStateRequest{
		Scope:        scope,
		Events:       append([]recordings.CanonicalEvent(nil), events...),
		SelectedTick: selectedTick,
	}
}

func canonicalEventsFromSubscription(
	ctx context.Context,
	subscription recordings.EventSubscription,
) ([]recordings.CanonicalEvent, error) {
	if subscription == nil {
		return nil, nil
	}
	events := make([]recordings.CanonicalEvent, 0)
	for {
		outcome := subscription.Next(ctx)
		switch outcome.Kind {
		case recordings.SubscriptionEvent:
			events = append(events, outcome.Event)
		case recordings.SubscriptionClosed:
			return events, nil
		case recordings.SubscriptionGap:
			if outcome.Gap == nil {
				return nil, errors.New("recordings subscription gap")
			}
			return nil, fmt.Errorf(
				"recordings subscription gap at sequence %d",
				outcome.Gap.ExpectedSequence,
			)
		default:
			return events, nil
		}
	}
}

func requireSnapshotContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("Work snapshot read context is required")
	}
	return ctx.Err()
}

func isPublicWorkItem(item work.FactoryWorkItem) bool {
	return !factorydefinitions.IsSystemTimeWorkType(item.WorkTypeID)
}

// Package sessionprojectionfacts contains the small, dependency-neutral
// contract shared by the recordings ledger and live Factory Session reads.
package sessionprojectionfacts

import (
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// SessionProjectionFacts contains the event-derived facts needed by live
// Factory Session reads.
type SessionProjectionFacts struct {
	PendingHumanApprovals map[string]interfaces.FactoryWorldHumanApproval
	JavaScriptRuntime     *interfaces.FactorySessionJavaScriptRuntimeState
	SessionBracket        *interfaces.FactoryWorldSessionBracketState
}

// CanonicalEventSequence is the Recordings-assigned global position of one
// event in canonical Factory-event order. A session-scoped selection can
// therefore contain increasing, non-contiguous values where other sessions'
// events occupy the intervening positions.
type CanonicalEventSequence int64

// CanonicalEventCursor is a portable reconnect position in global canonical
// order. StreamGenerationID distinguishes histories whose numeric sequences
// may overlap; SubscribeRequest.Scope selects which event at that position may
// be acknowledged.
type CanonicalEventCursor struct {
	StreamGenerationID string
	Sequence           CanonicalEventSequence
}

// WorkerSessionWorkFacts contains only selected Work and physical dispatch
// facts. Values are detached from the append-maintained projection.
type WorkerSessionWorkFacts struct {
	KnownWork          bool
	WorkName           string
	StreamGenerationID string
	World              interfaces.FactoryWorldState
	Associations       map[string]WorkerSessionAssociationFacts
	Requests           map[string]interfaces.FactoryWorldDispatch
	StateCursors       map[string]CanonicalEventCursor
	ResponseTimes      map[string]time.Time
	ResponseCursors    map[string]CanonicalEventCursor
	Interruptions      map[string]interfaces.DispatchInterruptedEventPayload
}

type WorkerSessionAssociationFacts struct {
	WorkerSessionID string
	TurnID          string
	Model           *string
	ReasoningEffort *string
	AssociatedAt    time.Time
}

// WorkOriginFacts selects immutable payload ancestry and exact dispatch
// associations from one ledger. Consumers retain policy ownership.
type WorkOriginFacts struct {
	InitialSnapshot              *work.WorkPayloadSnapshot
	ParentSnapshotsByID          map[string]work.WorkPayloadSnapshot
	WorkerSessionIDsByDispatchID map[string]string
	WorkerSessionID              string
	RelatedWorkIDs               []string
}

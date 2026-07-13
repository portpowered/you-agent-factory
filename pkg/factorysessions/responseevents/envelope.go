package responseevents

import (
	"encoding/json"
	"time"
)

// FactoryResponseEvent is the provider-neutral envelope for transient agent
// activity observed during one Factory Session run. Unlike canonical factory
// events, these records are ephemeral observation records and must not derive
// canonical work state after replay.
type FactoryResponseEvent struct {
	DispatchID         string          `json:"dispatchId,omitempty"`
	EventID            string          `json:"eventId"`
	FactorySessionID   string          `json:"factorySessionId"`
	ItemID             string          `json:"itemId,omitempty"`
	Kind               Kind            `json:"kind"`
	ParentItemID       string          `json:"parentItemId,omitempty"`
	Payload            json.RawMessage `json:"payload"`
	Phase              Phase           `json:"phase"`
	Provenance         Provenance      `json:"provenance"`
	ProviderSessionRef string          `json:"providerSessionRef,omitempty"`
	RecordedAt         time.Time       `json:"recordedAt"`
	RunID              string          `json:"runId"`
	SchemaVersion      string          `json:"schemaVersion"`
	Sequence           int64           `json:"sequence"`
	TurnID             string          `json:"turnId,omitempty"`
}

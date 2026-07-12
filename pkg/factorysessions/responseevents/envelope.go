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
	SchemaVersion    string          `json:"schemaVersion"`
	EventID          string          `json:"eventId"`
	Sequence         int64           `json:"sequence"`
	RecordedAt       time.Time       `json:"recordedAt"`
	FactorySessionID string          `json:"factorySessionId"`
	RunID            string          `json:"runId"`
	Kind             Kind            `json:"kind"`
	Phase            Phase           `json:"phase"`
	Provenance       Provenance      `json:"provenance"`
	Payload          json.RawMessage `json:"payload"`

	DispatchID         string `json:"dispatchId,omitempty"`
	TurnID             string `json:"turnId,omitempty"`
	ItemID             string `json:"itemId,omitempty"`
	ParentItemID       string `json:"parentItemId,omitempty"`
	ProviderSessionRef string `json:"providerSessionRef,omitempty"`
}

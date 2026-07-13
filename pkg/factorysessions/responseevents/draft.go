package responseevents

import "encoding/json"

// Draft is a provider-neutral response event before session-owned publication
// metadata is assigned. Adapters may describe semantic and correlation facts,
// but the response-event store remains responsible for event IDs, sequence,
// recorded time, and Factory Session identity.
type Draft struct {
	RunID      string          `json:"runId,omitempty"`
	Kind       Kind            `json:"kind"`
	Phase      Phase           `json:"phase"`
	Provenance Provenance      `json:"provenance"`
	Payload    json.RawMessage `json:"payload"`

	DispatchID         string `json:"dispatchId,omitempty"`
	TurnID             string `json:"turnId,omitempty"`
	ItemID             string `json:"itemId,omitempty"`
	ParentItemID       string `json:"parentItemId,omitempty"`
	ProviderSessionRef string `json:"providerSessionRef,omitempty"`
}

// ValidateDraft applies the canonical kind, phase, and payload rules without
// requiring publication metadata that an adapter does not own.
func ValidateDraft(draft Draft) error {
	return ValidateEvent(FactoryResponseEvent{
		Kind:    draft.Kind,
		Phase:   draft.Phase,
		Payload: draft.Payload,
	})
}

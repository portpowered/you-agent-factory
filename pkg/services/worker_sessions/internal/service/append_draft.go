package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// appendDraft validates draft with the existing Workers draft rules, then
// appends it, detached, onto topic using identity as the complete Events
// idempotency tuple. Every SESSION or source-native Worker record this
// package commits -- the W3 opening record and a caller's published source
// observation alike -- funnels through this one helper, so "validate with
// workers.ValidateDraft, then events.Append with a caller-owned identity" is
// defined in exactly one place. A non-nil error means no record was
// committed; draft is never marshaled or appended once ValidateDraft
// rejects it.
func (r *registry) appendDraft(ctx context.Context, topic events.Topic, identity events.AppendIdentity, schemaID events.SchemaID, draft workers.Draft) (events.AppendResult, error) {
	if err := workers.ValidateDraft(draft); err != nil {
		return events.AppendResult{}, fmt.Errorf("worker sessions: invalid draft: %w", err)
	}
	// draft's fields are exhaustively strings and json.RawMessage, neither of
	// which json.Marshal can fail to encode, so the marshal error is
	// unreachable and intentionally discarded rather than defended against.
	envelope, _ := json.Marshal(draft)
	return r.events.Append(ctx, events.AppendRequest{
		Topic:          topic,
		SourceType:     identity.SourceType,
		SourceID:       identity.SourceID,
		SourceSequence: identity.SourceSequence,
		SourceEventID:  identity.SourceEventID,
		SchemaID:       schemaID,
		Payload:        envelope,
	}.Detached())
}

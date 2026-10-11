package admission

import (
	"context"
	"encoding/json"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	"github.com/portpowered/infinite-you/pkg/services/events"
)

// persist is called under the admission lock so observations preserve commit
// order, including atomic reply and batch inbox changes. The journal alone
// decides success: an unavailable transient stream cannot undo a durable send
// or turn a successful request retry into another admission.
func (e *Engine) persist(ctx context.Context, t store.Transaction) error {
	if err := e.ledger.Commit(t); err != nil {
		return err
	}
	if e.events == nil {
		return nil
	}
	// A disconnected client may leave a durable success. Publish that success
	// independently of its request cancellation; there are no external calls.
	publication := context.WithoutCancel(ctx)
	for _, entry := range t.Messages {
		observation := agentmessages.Observation{RecordID: t.RecordID, Sequence: t.Sequence,
			Kind: observationKind(entry), Message: entry.Message}
		payload, err := json.Marshal(observation)
		if err == nil {
			_, err = e.events.Append(publication, events.AppendRequest{
				Topic: agentmessages.ObservationTopic, SourceType: "agent-message",
				SourceID: events.SourceID(entry.Message.MessageID), SourceSequence: events.SourceSequence(t.Sequence),
				SourceEventID: events.SourceEventID(t.RecordID), SchemaID: agentmessages.ObservationSchema, Payload: payload,
			})
		}
		if err != nil && e.logger != nil {
			// Collaborator diagnostics may contain credentials or payloads. Only
			// the stable outcome is safe to publish to operational telemetry.
			e.logger.WarnContext(publication, "Agent Message observation unavailable")
		}
	}
	return nil
}

func observationKind(entry store.Entry) string {
	if entry.Message.Status == agentmessages.Queued {
		return store.Sent
	}
	return string(entry.Message.Status)
}

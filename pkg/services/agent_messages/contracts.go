package agentmessages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// ObservationTopic identifies the process-local stream for an admitted exact
// recipient. The tuple preserves legacy Factory Session scope without delimiter
// collisions; its digest bounds opaque IDs to Events' topic alphabet and length.
// A topic grants no read authority. Messaging must authorize every observation.
func (recipient Recipient) ObservationTopic() events.Topic {
	identity, _ := json.Marshal([2]string{recipient.FactorySessionID, recipient.WorkerSessionID})
	digest := sha256.Sum256(identity)
	return events.Topic("messages/" + hex.EncodeToString(digest[:]))
}

const ObservationSchema events.SchemaID = "agent-message-observation/v1"

// ObservationStream aggregates recipient streams for authorized live queries.
// Transports consume it only through Service.Follow; Events topic names are
// routing identities, never customer authorization capabilities.
const ObservationStream events.Topic = "agent-messages/events"

// Service is the sole peer boundary for durable Agent Message operations.
// Caller credentials are execution-only and are validated on every operation.
// A nil caller permits operator observation, never send, reply or mark-read.
type Service interface {
	Send(context.Context, SendRequest, *workersessions.CallerIdentity, string) (Message, error)
	Get(context.Context, GetRequest) (Message, error)
	List(context.Context, ListRequest) (Page, error)
	// Follow invokes the observer only for authorized, matching source-native
	// observations. Cancellation or owner loss ends this invocation's stream.
	Follow(context.Context, ListRequest, func(Observation) error) error
}

// GetRequest names one message. Only its permitted recipient may durably mark
// a QUEUED message READ; sender/operator observation leaves its state intact.
type GetRequest struct {
	MessageID string
	Caller    *workersessions.CallerIdentity `json:"-"`
}

// ListRequest applies AND filters after authorization. FactorySessionID selects
// legacy address resolution only; Correlation selects descriptive message facts.
type ListRequest struct {
	Caller              *workersessions.CallerIdentity `json:"-"`
	FactorySessionID    string
	ToMe                bool
	ToWorkerSessionID   string
	FromWorkerSessionID string
	ThreadID            string
	Correlation         Correlation
	Statuses            []Status
	MaxResults          int
	NextToken           string
	MarkRead            bool
}

// Page is ordered by original committed send sequence. NextToken is bound to
// the authorized caller and query, and never grants read authority on its own.
type Page struct {
	Messages  []Message `json:"messages"`
	NextToken string    `json:"nextToken,omitempty"`
}

// Observation belongs to the process-local Events stream, independently of
// canonical Factory Events. Message contains privacy-normalized content only.
type Observation struct {
	RecordID string  `json:"recordId"`
	Sequence uint64  `json:"sequence"`
	Kind     string  `json:"kind"`
	Message  Message `json:"message"`
}

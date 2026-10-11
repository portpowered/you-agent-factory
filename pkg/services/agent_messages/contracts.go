package agentmessages

import (
	"context"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

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

// Package agentmessages owns Agent Message admission, durable conversations,
// and inbox reads independently of Factory execution.
package agentmessages

import "time"

// Status is a durable inbox state. T5 admits only queued delivery.
type Status string

const (
	Queued  Status = "QUEUED"
	Read    Status = "READ"
	Replied Status = "REPLIED"
	Expired Status = "EXPIRED"
)

// Sender identifies the authenticated Worker that admitted a message.
type Sender struct {
	Principal        string `json:"principal"`
	WorkerSessionID  string `json:"workerSessionId"`
	FactorySessionID string `json:"factorySessionId,omitempty"`
}

// Recipient retains the exact addressed owner, independently of continuation.
type Recipient struct {
	Kind                     string `json:"kind"`
	WorkerSessionID          string `json:"workerSessionId"`
	FactorySessionID         string `json:"factorySessionId,omitempty"`
	DeliveredWorkerSessionID string `json:"deliveredWorkerSessionId,omitempty"`
}

// Correlation is descriptive context; it never grants message authority.
type Correlation struct {
	WorkID           string `json:"workId,omitempty"`
	FactorySessionID string `json:"factorySessionId,omitempty"`
}

// Message contains privacy-normalized content only. Credentials are not part
// of the representation, durable state, or request fingerprint.
type Message struct {
	MessageID              string      `json:"messageId"`
	ThreadID               string      `json:"threadId"`
	InReplyTo              string      `json:"inReplyTo,omitempty"`
	From                   Sender      `json:"from"`
	To                     Recipient   `json:"to"`
	Correlation            Correlation `json:"correlation,omitempty"`
	Body                   string      `json:"body"`
	BodySHA256             string      `json:"bodySha256"`
	BodyRedactionCount     int         `json:"bodyRedactionCount"`
	Delivery               string      `json:"delivery"`
	IfEnded                string      `json:"ifEnded"`
	ReplyIfEnded           string      `json:"replyIfEnded"`
	Status                 Status      `json:"status"`
	Reason                 string      `json:"reason,omitempty"`
	Hop                    int         `json:"hop"`
	SentAt                 time.Time   `json:"sentAt"`
	ExpiresAt              time.Time   `json:"expiresAt"`
	RevivedWorkerSessionID string      `json:"revivedWorkerSessionId,omitempty"`
	RepliedByMessageID     string      `json:"repliedByMessageId,omitempty"`
}

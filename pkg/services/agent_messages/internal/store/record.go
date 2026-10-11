// Package store owns the private, versioned Agent Message transaction journal.
// One transaction contains all message changes and request aliases for an
// admission; publishing an in-memory change before its durable flush is forbidden.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

const (
	Version      = 1
	Sent         = "SENT"
	Read         = "READ"
	Replied      = "REPLIED"
	Expired      = "EXPIRED"
	RequestAlias = "REQUEST_ALIAS"
	Snapshot     = "SNAPSHOT"
	maxBodyBytes = 8192
)

var (
	ErrUnavailable        = errors.New("MESSAGE_STORE_UNAVAILABLE")
	ErrCorrupt            = errors.New("MESSAGE_STORE_CORRUPT")
	ErrInvalidTransaction = errors.New("MESSAGE_STORE_INVALID_TRANSACTION")
)

// Entry adds the verified identities needed for inbox reconstruction and quota
// accounting. These are admission facts, never caller-supplied labels.
type Entry struct {
	Message                agentmessages.Message `json:"message"`
	SenderChainIdentity    string                `json:"senderChainIdentity"`
	RecipientChainIdentity string                `json:"recipientChainIdentity"`
	SenderWorkIdentity     string                `json:"senderWorkIdentity,omitempty"`
	RecipientWorkIdentity  string                `json:"recipientWorkIdentity,omitempty"`
	Sequence               uint64                `json:"sequence"`
}

// Request persists both original request keys and identical-body aliases.
// RequestSHA256 fingerprints only the already privacy-normalized request.
type Request struct {
	SenderIdentity string `json:"senderIdentity"`
	RequestID      string `json:"requestId"`
	RequestSHA256  string `json:"requestSha256"`
	MessageID      string `json:"messageId"`
}

// Transaction is the v1 serializer contract. Reply admission changes its
// parent and child together; an alias record changes no message state.
type Transaction struct {
	Version     int       `json:"version"`
	RecordID    string    `json:"recordId"`
	Sequence    uint64    `json:"sequence"`
	Kind        string    `json:"kind"`
	CommittedAt time.Time `json:"committedAt"`
	Messages    []Entry   `json:"messages"`
	Requests    []Request `json:"requests"`
}

func (t Transaction) validate() error {
	if t.Version != Version || !nonempty(t.RecordID) || t.Sequence == 0 || t.CommittedAt.IsZero() ||
		t.Messages == nil || t.Requests == nil {
		return ErrInvalidTransaction
	}
	switch t.Kind {
	case Sent, Read, Replied, Expired, RequestAlias, Snapshot:
	default:
		return ErrInvalidTransaction
	}
	for _, entry := range t.Messages {
		if !entry.valid() {
			return ErrInvalidTransaction
		}
	}
	for _, request := range t.Requests {
		if !request.valid() {
			return ErrInvalidTransaction
		}
	}
	return nil
}

func (request Request) valid() bool {
	return nonempty(request.SenderIdentity) && nonempty(request.RequestID) &&
		len(request.RequestID) <= 200 && nonempty(request.MessageID) && validHash(request.RequestSHA256)
}

func (entry Entry) valid() bool {
	m := entry.Message
	if !nonempty(entry.SenderChainIdentity) || !nonempty(entry.RecipientChainIdentity) || entry.Sequence == 0 ||
		!nonempty(m.MessageID) || !nonempty(m.ThreadID) {
		return false
	}
	return validAddress(m) && validDelivery(m) && validContent(m)
}

func validAddress(m agentmessages.Message) bool {
	if m.From.Principal != "WORKER" || m.To.Kind != "WORKER_SESSION" ||
		!nonempty(m.From.WorkerSessionID) || !nonempty(m.To.WorkerSessionID) {
		return false
	}
	if m.From.WorkerSessionID == m.To.WorkerSessionID && m.From.FactorySessionID == m.To.FactorySessionID {
		return false
	}
	return true
}

func validDelivery(m agentmessages.Message) bool {
	if m.Delivery != "QUEUE" || !endedPolicy(m.IfEnded) || !endedPolicy(m.ReplyIfEnded) ||
		m.Hop < 0 || m.Hop > 3 || m.SentAt.IsZero() || m.RevivedWorkerSessionID != "" {
		return false
	}
	expiry := m.ExpiresAt.Sub(m.SentAt)
	if expiry < time.Minute || expiry > 7*24*time.Hour {
		return false
	}
	if m.Reason != "" && m.Reason != "REVIVE_UNSUPPORTED" {
		return false
	}
	switch m.Status {
	case agentmessages.Queued, agentmessages.Read, agentmessages.Replied, agentmessages.Expired:
		return true
	default:
		return false
	}
}

func validContent(m agentmessages.Message) bool {
	if !utf8.ValidString(m.Body) || !nonempty(m.Body) || len(m.Body) > maxBodyBytes || m.BodyRedactionCount < 0 {
		return false
	}
	digest := sha256.Sum256([]byte(m.Body))
	return m.BodySHA256 == hex.EncodeToString(digest[:])
}

func endedPolicy(value string) bool { return value == "REVIVE" || value == "HOLD" }
func nonempty(value string) bool    { return utf8.ValidString(value) && strings.TrimSpace(value) != "" }
func validHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

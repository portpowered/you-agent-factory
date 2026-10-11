package admission

import (
	"context"
	"sync"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// Authority supplies live authentication and server-held relationship facts.
type Authority interface {
	Authenticate(context.Context, *workersessions.CallerIdentity) (Identity, error)
	Recipient(context.Context, string, string) (Identity, error)
	PermitSend(context.Context, Identity, Identity) error
	Revalidate(context.Context, *workersessions.CallerIdentity) error
}

type InputValidator interface {
	Prepare(agentmessages.SendRequest, string) (Prepared, error)
}

type Quota interface {
	Check(time.Time, string, string, int, []store.Entry) error
}

// Ledger's commit atomically flushes all changes before making them readable.
type Ledger interface {
	Entries() ([]store.Entry, uint64, error)
	LookupRequest(string, string) (store.Request, bool, error)
	Commit(store.Transaction) error
}

// Engine serializes admission across policy, request aliases, limits and the
// durable transaction. It retains no credentials. Wire supplies one instance
// per writable profile and injects controlled effects at these exact ports.
type Engine struct {
	mu        sync.Mutex
	enabled   bool
	authority Authority
	validator InputValidator
	quota     Quota
	ledger    Ledger
	now       func() time.Time
	newID     func() string
	cursorKey []byte
}

func NewEngine(enabled bool, authority Authority, validator InputValidator, quota Quota, ledger Ledger, now func() time.Time, newID func() string, cursorKey []byte) *Engine {
	return &Engine{enabled: enabled, authority: authority, validator: validator, quota: quota, ledger: ledger, now: now, newID: newID, cursorKey: append([]byte(nil), cursorKey...)}
}

func (e *Engine) Send(ctx context.Context, request agentmessages.SendRequest, caller *workersessions.CallerIdentity, factory string) (agentmessages.Message, error) {
	if !e.enabled {
		return agentmessages.Message{}, agentmessages.ErrDisabled
	}
	if caller == nil {
		return agentmessages.Message{}, agentmessages.ErrNotPermitted
	}
	// Detach credentials before collaborating code or a waiting admission can
	// observe a caller-owned mutation. Authentication precedes payload work.
	caller = caller.Clone()
	sender, err := e.authority.Authenticate(ctx, caller)
	if err != nil {
		return agentmessages.Message{}, err
	}
	prepared, err := e.validator.Prepare(request, caller.Token)
	if err != nil {
		return agentmessages.Message{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	entries, sequence, err := e.ledger.Entries()
	if err != nil {
		return agentmessages.Message{}, err
	}
	now := e.now().UTC()
	parent, err := replyParent(prepared.Request, sender, entries)
	if err != nil {
		return agentmessages.Message{}, err
	}
	prepared, factory = replyDefaults(prepared, parent, factory)
	recipient, err := e.authority.Recipient(ctx, prepared.Request.To.WorkerSessionID, factory)
	if err != nil {
		return agentmessages.Message{}, err
	}
	if err := e.authority.PermitSend(ctx, sender, recipient); err != nil {
		return agentmessages.Message{}, err
	}
	fingerprint, err := prepared.Fingerprint(recipient.Observation.FactorySessionID)
	if err != nil {
		return agentmessages.Message{}, err
	}
	alias := store.Request{SenderIdentity: sender.Chain, RequestID: prepared.Request.RequestID, RequestSHA256: fingerprint}
	prior, exists, err := e.ledger.LookupRequest(alias.SenderIdentity, alias.RequestID)
	if err != nil {
		return agentmessages.Message{}, err
	}
	if exists {
		return e.retry(ctx, caller, prior, fingerprint, entries)
	}
	if duplicate, found := identicalMessage(now, prepared, sender, recipient, entries); found {
		alias.MessageID = duplicate.MessageID
		t := e.transaction(sequence, now, store.RequestAlias, nil, alias)
		if err := e.commit(ctx, caller, t); err != nil {
			return agentmessages.Message{}, err
		}
		return duplicate, nil
	}
	return e.admit(ctx, caller, prepared, sender, recipient, parent, entries, sequence, now, alias)
}

func (e *Engine) retry(ctx context.Context, caller *workersessions.CallerIdentity, prior store.Request, fingerprint string, entries []store.Entry) (agentmessages.Message, error) {
	if prior.RequestSHA256 != fingerprint {
		return agentmessages.Message{}, agentmessages.ErrRequestConflict
	}
	if err := e.authority.Revalidate(ctx, caller); err != nil {
		return agentmessages.Message{}, err
	}
	return priorMessage(prior.MessageID, entries)
}

func (e *Engine) commit(ctx context.Context, caller *workersessions.CallerIdentity, t store.Transaction) error {
	// This is the authorization linearization point. A previously observed
	// terminal/owner-lost caller cannot reach persistence or spend quota.
	if err := e.authority.Revalidate(ctx, caller); err != nil {
		return err
	}
	return e.ledger.Commit(t)
}

func (e *Engine) transaction(sequence uint64, now time.Time, kind string, entries []store.Entry, alias store.Request) store.Transaction {
	if entries == nil {
		entries = []store.Entry{}
	}
	return store.Transaction{Version: store.Version, RecordID: e.newID(), Sequence: sequence + 1,
		Kind: kind, CommittedAt: now, Messages: entries, Requests: []store.Request{alias}}
}

func priorMessage(id string, entries []store.Entry) (agentmessages.Message, error) {
	for _, entry := range entries {
		if entry.Message.MessageID == id {
			return entry.Message, nil
		}
	}
	return agentmessages.Message{}, store.ErrCorrupt
}

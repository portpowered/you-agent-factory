package admission

import (
	"context"
	"sort"
	"strings"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
)

const (
	defaultPageSize = 100
	maxPageSize     = 500
)

// List filters authorized admission facts in original send order. The cursor
// fixes a send-sequence ceiling so concurrent admissions cannot extend a page
// walk indefinitely. Statuses remain current, rather than a historical snapshot.
func (e *Engine) List(ctx context.Context, request agentmessages.ListRequest) (agentmessages.Page, error) {
	if !e.enabled {
		return agentmessages.Page{}, agentmessages.ErrDisabled
	}
	request.Caller = request.Caller.Clone()
	var identity Identity
	if request.Caller != nil {
		var err error
		identity, err = e.authority.Authenticate(ctx, request.Caller)
		if err != nil {
			return agentmessages.Page{}, err
		}
	} else if request.ToMe || request.MarkRead {
		return agentmessages.Page{}, agentmessages.ErrNotPermitted
	}
	request, err := normalizedList(request)
	if err != nil {
		return agentmessages.Page{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	filter := store.Filter{ToWorkerSessionID: request.ToWorkerSessionID,
		FromWorkerSessionID: request.FromWorkerSessionID, ThreadID: request.ThreadID,
		WorkID: request.Correlation.WorkID, FactorySessionID: request.Correlation.FactorySessionID}
	if request.Caller != nil {
		filter.ReaderChain, filter.ReaderWork, filter.RecipientOnly = identity.Chain, identity.Work, request.ToMe
	}
	entries, sequence, err := e.ledger.Matching(filter)
	if err != nil {
		return agentmessages.Page{}, err
	}
	cursor, err := e.pageCursor(request, identity, sequence)
	if err != nil {
		return agentmessages.Page{}, err
	}
	now := e.now().UTC()
	selected, more := selectPage(entries, request, identity, cursor, now)
	page := agentmessages.Page{Messages: make([]agentmessages.Message, 0, len(selected))}
	if more {
		cursor.After = selected[len(selected)-1].Sequence
		page.NextToken, err = e.encodeCursor(cursor)
		if err != nil {
			return agentmessages.Page{}, err
		}
	}
	if err := e.updatePage(ctx, request, identity, selected, sequence, now); err != nil {
		return agentmessages.Page{}, err
	}
	for _, entry := range selected {
		page.Messages = append(page.Messages, entry.Message)
	}
	return page, nil
}

func normalizedList(r agentmessages.ListRequest) (agentmessages.ListRequest, error) {
	if !validListOptions(r) {
		return r, agentmessages.ErrBadRequest
	}
	if r.MaxResults == 0 {
		r.MaxResults = defaultPageSize
	}
	statuses := make(map[agentmessages.Status]bool)
	for _, status := range r.Statuses {
		switch status {
		case agentmessages.Queued, agentmessages.Read, agentmessages.Replied, agentmessages.Expired,
			"DELIVERED_REVIVE", "DELIVERED_INTERRUPT", "REJECTED":
			statuses[status] = true
		default:
			return r, agentmessages.ErrBadRequest
		}
	}
	r.Statuses = make([]agentmessages.Status, 0, len(statuses))
	for status := range statuses {
		r.Statuses = append(r.Statuses, status)
	}
	sort.Slice(r.Statuses, func(i, j int) bool { return r.Statuses[i] < r.Statuses[j] })
	return r, nil
}

func validListOptions(r agentmessages.ListRequest) bool {
	if r.MaxResults < 0 || r.MaxResults > maxPageSize {
		return false
	}
	for _, value := range []string{r.FactorySessionID, r.ToWorkerSessionID, r.FromWorkerSessionID,
		r.ThreadID, r.Correlation.WorkID, r.Correlation.FactorySessionID} {
		if !optionalID(value) {
			return false
		}
		if r.Caller != nil && r.Caller.Token != "" && strings.Contains(value, r.Caller.Token) {
			return false
		}
	}
	return true
}

func selectPage(entries []store.Entry, r agentmessages.ListRequest, identity Identity, cursor pagePosition, now time.Time) ([]store.Entry, bool) {
	selected := make([]store.Entry, 0, r.MaxResults)
	for _, entry := range entries {
		if entry.Sequence <= cursor.After || entry.Sequence > cursor.Through || !visibleEntry(r, identity, entry) {
			continue
		}
		// Expiry participates in status filtering immediately; mark-read happens
		// after filtering so --status QUEUED --mark-read selects queued entries.
		current := entry.Message
		if readTransition(now, false, current) == store.Expired {
			current.Status = agentmessages.Expired
		}
		if !matchesList(r, current) {
			continue
		}
		if len(selected) == r.MaxResults {
			return selected, true
		}
		selected = append(selected, entry)
	}
	return selected, false
}

func visibleEntry(r agentmessages.ListRequest, identity Identity, entry store.Entry) bool {
	recipient := identity.Matches(entry.RecipientChainIdentity, entry.RecipientWorkIdentity)
	if r.ToMe {
		return recipient
	}
	return r.Caller == nil || recipient || identity.Matches(entry.SenderChainIdentity, entry.SenderWorkIdentity)
}

func matchesList(r agentmessages.ListRequest, m agentmessages.Message) bool {
	if (r.ToWorkerSessionID != "" && r.ToWorkerSessionID != m.To.WorkerSessionID) ||
		(r.FromWorkerSessionID != "" && r.FromWorkerSessionID != m.From.WorkerSessionID) ||
		(r.ThreadID != "" && r.ThreadID != m.ThreadID) ||
		(r.Correlation.WorkID != "" && r.Correlation.WorkID != m.Correlation.WorkID) ||
		(r.Correlation.FactorySessionID != "" && r.Correlation.FactorySessionID != m.Correlation.FactorySessionID) {
		return false
	}
	if len(r.Statuses) == 0 {
		return true
	}
	for _, status := range r.Statuses {
		if status == m.Status {
			return true
		}
	}
	return false
}

func (e *Engine) updatePage(ctx context.Context, r agentmessages.ListRequest, identity Identity, selected []store.Entry, sequence uint64, now time.Time) error {
	changes := make([]store.Entry, 0, len(selected))
	for i := range selected {
		entry := &selected[i]
		recipient := r.MarkRead && identity.Matches(entry.RecipientChainIdentity, entry.RecipientWorkIdentity)
		kind := readTransition(now, recipient, entry.Message)
		if kind != "" {
			entry.Message.Status = agentmessages.Status(kind)
			changes = append(changes, *entry)
		}
	}
	if err := e.readAuthority(ctx, r.Caller); err != nil {
		return err
	}
	if len(changes) == 0 {
		return nil
	}
	t := store.Transaction{Version: store.Version, RecordID: e.newID(), Sequence: sequence + 1,
		Kind: store.InboxUpdated, CommittedAt: now, Messages: changes, Requests: []store.Request{}}
	return e.persist(ctx, t)
}

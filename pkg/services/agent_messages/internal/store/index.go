package store

import "sort"

// Filter selects immutable address and correlation facts. It grants no read
// authority; the admission coordinator still authorizes every returned entry
// and applies current status filters and page boundaries.
type Filter struct {
	ToWorkerSessionID   string
	FromWorkerSessionID string
	ThreadID            string
	WorkID              string
	FactorySessionID    string
	ReaderChain         string
	ReaderWork          string
	RecipientOnly       bool
}

type indexKey struct{ field, value string }

func (f Filter) keys() []indexKey {
	return []indexKey{{"to", f.ToWorkerSessionID}, {"from", f.FromWorkerSessionID},
		{"thread", f.ThreadID}, {"work", f.WorkID}, {"factory", f.FactorySessionID}}
}

func entryFilter(e Entry) Filter {
	m := e.Message
	return Filter{ToWorkerSessionID: m.To.WorkerSessionID, FromWorkerSessionID: m.From.WorkerSessionID,
		ThreadID: m.ThreadID, WorkID: m.Correlation.WorkID, FactorySessionID: m.Correlation.FactorySessionID}
}

func (j *Journal) index(entry Entry) {
	id := entry.Message.MessageID
	j.ordered = append(j.ordered, id)
	for _, key := range entryFilter(entry).keys() {
		if key.value != "" {
			j.indexes[key] = append(j.indexes[key], id)
		}
	}
	for _, key := range []indexKey{{"recipientChain", entry.RecipientChainIdentity},
		{"recipientWork", entry.RecipientWorkIdentity}, {"senderChain", entry.SenderChainIdentity},
		{"senderWork", entry.SenderWorkIdentity}} {
		if key.value != "" {
			j.indexes[key] = append(j.indexes[key], id)
		}
	}
}

// LookupMessage returns detached current state and the transaction sequence
// without copying or walking the inbox. No reference to mutable index storage
// escapes the journal lock.
func (j *Journal) LookupMessage(id string) (Entry, bool, uint64, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.opened || j.fault != nil {
		return Entry{}, false, 0, j.unavailable()
	}
	entry, exists := j.entries[id]
	return entry, exists, j.sequence, nil
}

// Matching walks the smallest applicable immutable index and intersects all
// filters. Status changes never append duplicate index entries; reconstruction
// rebuilds the same original admission order before reads become available.
func (j *Journal) Matching(filter Filter) ([]Entry, uint64, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.opened || j.fault != nil {
		return nil, 0, j.unavailable()
	}
	ids := j.ordered
	if filter.ReaderChain != "" || filter.ReaderWork != "" {
		ids = j.audience(filter)
	}
	for _, key := range filter.keys() {
		if key.value != "" && len(j.indexes[key]) < len(ids) {
			ids = j.indexes[key]
		}
	}
	result := make([]Entry, 0, len(ids))
	for _, id := range ids {
		entry := j.entries[id]
		if matchesFilter(filter, entryFilter(entry)) && matchesAudience(filter, entry) {
			result = append(result, entry)
		}
	}
	return result, j.sequence, nil
}

func (j *Journal) audience(filter Filter) []string {
	keys := []indexKey{{"recipientChain", filter.ReaderChain}, {"recipientWork", filter.ReaderWork}}
	if !filter.RecipientOnly {
		keys = append(keys, indexKey{"senderChain", filter.ReaderChain}, indexKey{"senderWork", filter.ReaderWork})
	}
	seen := make(map[string]bool)
	ids := []string{}
	for _, key := range keys {
		if key.value == "" {
			continue
		}
		for _, id := range j.indexes[key] {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	sort.Slice(ids, func(a, b int) bool { return j.entries[ids[a]].Sequence < j.entries[ids[b]].Sequence })
	return ids
}

func matchesAudience(f Filter, e Entry) bool {
	if f.ReaderChain == "" && f.ReaderWork == "" {
		return true
	}
	recipient := (f.ReaderChain != "" && f.ReaderChain == e.RecipientChainIdentity) ||
		(f.ReaderWork != "" && f.ReaderWork == e.RecipientWorkIdentity)
	sender := (f.ReaderChain != "" && f.ReaderChain == e.SenderChainIdentity) ||
		(f.ReaderWork != "" && f.ReaderWork == e.SenderWorkIdentity)
	return recipient || (!f.RecipientOnly && sender)
}

func matchesFilter(want, got Filter) bool {
	return (want.ToWorkerSessionID == "" || want.ToWorkerSessionID == got.ToWorkerSessionID) &&
		(want.FromWorkerSessionID == "" || want.FromWorkerSessionID == got.FromWorkerSessionID) &&
		(want.ThreadID == "" || want.ThreadID == got.ThreadID) &&
		(want.WorkID == "" || want.WorkID == got.WorkID) &&
		(want.FactorySessionID == "" || want.FactorySessionID == got.FactorySessionID)
}

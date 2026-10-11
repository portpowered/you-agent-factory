package store

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

func assertMatches(t *testing.T, journal *Journal, filter Filter, sequence uint64, ids ...string) {
	t.Helper()
	entries, gotSequence, err := journal.Matching(filter)
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Message.MessageID)
	}
	if err != nil || gotSequence != sequence || !slices.Equal(got, ids) {
		t.Fatalf("matching %+v = %v, sequence %d, error %v; want %v at %d", filter, got, gotSequence, err, ids, sequence)
	}
}

// The journal is the isolated component under test: real files protect durable
// reconstruction, while controlled descriptors expose pre-publication faults.
func TestJournalIndexedFiltersIntersectAndRecoverCurrentState(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	j := newTestJournal(t, testFiles{}, path)
	for i, id := range []string{"first", "second", "third"} {
		tx := sentTransaction(id, uint64(i+1))
		tx.Messages[0].Message.Correlation = agentmessages.Correlation{WorkID: "work", FactorySessionID: "factory"}
		if id == "second" {
			tx.Messages[0].Message.To.WorkerSessionID = "other"
		}
		if id == "third" {
			tx.Messages[0].Message.Correlation.WorkID = "other-work"
		}
		mustCommit(t, j, tx)
	}
	entry, exists, seq, err := j.LookupMessage("first")
	if err != nil || !exists || seq != 3 {
		t.Fatal("lookup failed", err)
	}
	entry.Message.Status = agentmessages.Read
	mustCommit(t, j, transaction(4, Read, entry))
	for _, journal := range []*Journal{j, newTestJournal(t, testFiles{}, path)} {
		assertMatches(t, journal, Filter{}, 4, "first", "second", "third")
		assertMatches(t, journal, Filter{ToWorkerSessionID: "recipient"}, 4, "first", "third")
		assertMatches(t, journal, Filter{WorkID: "work"}, 4, "first", "second")
		assertMatches(t, journal, Filter{ToWorkerSessionID: "recipient", FromWorkerSessionID: "sender",
			ThreadID: "first", WorkID: "work", FactorySessionID: "factory"}, 4, "first")
		assertMatches(t, journal, Filter{ThreadID: "second", ToWorkerSessionID: "recipient"}, 4)
		assertMatches(t, journal, Filter{FactorySessionID: "missing"}, 4)
		assertDetachedLookup(t, journal, entry)
	}
}

func assertDetachedLookup(t *testing.T, journal *Journal, entry Entry) {
	t.Helper()
	got, found, sequence, err := journal.LookupMessage("first")
	if err != nil || !found || sequence != 4 || got != entry {
		t.Fatal("lookup lost current detached state", err)
	}
	got.Message.Body = "caller mutation"
	entries, _, err := journal.Matching(Filter{ThreadID: "first"})
	if err != nil || len(entries) != 1 || entries[0] != entry {
		t.Fatal("lookup result mutated index", err)
	}
	entries[0].Message.Body = "page mutation"
	again, _, _, err := journal.LookupMessage("first")
	if err != nil || again != entry {
		t.Fatal("page result mutated index", err)
	}
	_, found, sequence, err = journal.LookupMessage("missing")
	if err != nil || found || sequence != 4 {
		t.Fatal("missing lookup fabricated a record", err)
	}
}

func TestJournalSnapshotIndexesPreserveOriginalOrder(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	j := newTestJournal(t, testFiles{}, path)
	tx := transaction(20, Snapshot, message("last", 17), message("first", 2), message("middle", 9))
	before := append([]Entry{}, tx.Messages...)
	mustCommit(t, j, tx)
	if !reflect.DeepEqual(tx.Messages, before) {
		t.Fatal("snapshot commit reordered caller input")
	}
	mustCommit(t, j, sentTransaction("next", 21))
	for _, journal := range []*Journal{j, newTestJournal(t, testFiles{}, path)} {
		assertMatches(t, journal, Filter{ToWorkerSessionID: "recipient"}, 21, "first", "middle", "last", "next")
		entries, seq, err := journal.Entries()
		if err != nil || seq != 21 || len(entries) != 4 || entries[0].Sequence != 2 || entries[3].Sequence != 21 {
			t.Fatal("snapshot order lost", err)
		}
	}
}

func TestJournalFailedAdmissionPublishesNoIndexEntry(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	files := &testFiles{}
	j := newTestJournal(t, files, path)
	mustCommit(t, j, sentTransaction("first", 1))
	files.open = func(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
		file, err := os.OpenFile(path, flags, mode)
		if err != nil {
			return nil, err
		}
		return &faultFile{File: file, fault: "short"}, nil
	}
	if err := j.Commit(sentTransaction("failed", 2)); !errors.Is(err, ErrUnavailable) {
		t.Fatal("short write admitted a record", err)
	}
	for _, journal := range []*Journal{j, newTestJournal(t, testFiles{}, path)} {
		assertMatches(t, journal, Filter{ToWorkerSessionID: "recipient"}, 1, "first")
		assertMatches(t, journal, Filter{ThreadID: "failed"}, 1)
		_, found, seq, err := journal.LookupMessage("failed")
		if err != nil || found || seq != 1 {
			t.Fatal("failed admission is readable", err)
		}
	}
}

func TestJournalIdentityIndexesUnionWithoutDuplicates(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	j := newTestJournal(t, testFiles{}, path)
	first := sentTransaction("first", 1)
	first.Messages[0].RecipientWorkIdentity = "own-work"
	mustCommit(t, j, first)
	second := sentTransaction("second", 2)
	second.Messages[0].RecipientChainIdentity = "continued-chain"
	second.Messages[0].RecipientWorkIdentity = "own-work"
	mustCommit(t, j, second)
	third := sentTransaction("third", 3)
	third.Messages[0].SenderChainIdentity = "recipient-chain"
	third.Requests[0].SenderIdentity = "recipient-chain"
	third.Messages[0].RecipientChainIdentity = "foreign-chain"
	mustCommit(t, j, third)
	for _, journal := range []*Journal{j, newTestJournal(t, testFiles{}, path)} {
		assertMatches(t, journal, Filter{ReaderChain: "recipient-chain", ReaderWork: "own-work"}, 3, "first", "second", "third")
		assertMatches(t, journal, Filter{ReaderChain: "recipient-chain", ReaderWork: "own-work", RecipientOnly: true}, 3, "first", "second")
		assertMatches(t, journal, Filter{ReaderChain: "foreign-chain", ReaderWork: "foreign-work"}, 3, "third")
		assertMatches(t, journal, Filter{ReaderChain: "missing"}, 3)
		assertMatches(t, journal, Filter{ReaderChain: "missing", ThreadID: "first"}, 3)
		assertMatches(t, journal, Filter{ReaderChain: "recipient-chain", ThreadID: "second"}, 3)
		assertMatches(t, journal, Filter{ReaderChain: "continued-chain", ThreadID: "second"}, 3, "second")
	}
}

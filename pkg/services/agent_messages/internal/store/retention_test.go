package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

func retentionFixture() (Transaction, time.Time) {
	parent, dropped, reply := message("parent", 1), message("dropped", 2), message("reply", 3)
	now := parent.Message.SentAt.Add(40 * 24 * time.Hour)
	parent.Message.Status, parent.Message.RepliedByMessageID = agentmessages.Replied, "reply"
	reply.Message.ThreadID, reply.Message.InReplyTo = "parent", "parent"
	reply.Message.Status = agentmessages.Expired
	reply.Message.SentAt, reply.Message.ExpiresAt = now.Add(-24*time.Hour), now
	dropped.Message.Status = agentmessages.Expired
	pending, boundary := message("pending", 4), message("boundary", 5)
	boundary.Message.Status = agentmessages.Expired
	boundary.Message.SentAt = now.Add(-DefaultRetention)
	boundary.Message.ExpiresAt = boundary.Message.SentAt.Add(24 * time.Hour)
	tx := transaction(10, Snapshot, parent, dropped, reply, pending, boundary)
	for _, id := range []string{"parent", "dropped", "reply", "pending", "boundary", "parent"} {
		tx.Requests = append(tx.Requests, Request{SenderIdentity: "sender-chain",
			RequestID: "request-" + id, RequestSHA256: strings.Repeat("a", 64), MessageID: id})
	}
	tx.Requests[len(tx.Requests)-1].RequestID = "alias-parent"
	return tx, now
}

func TestJournalRetentionPreservesThreadsAliasesAndOrder(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	j := newTestJournal(t, testFiles{}, path)
	tx, now := retentionFixture()
	mustCommit(t, j, tx)
	if err := j.Compact(now, DefaultRetention); err != nil {
		t.Fatal(err)
	}
	for _, journal := range []*Journal{j, newTestJournal(t, testFiles{}, path)} {
		assertMatches(t, journal, Filter{}, 10, "parent", "reply", "pending", "boundary")
		assertMatches(t, journal, Filter{ThreadID: "dropped"}, 10)
		assertMatches(t, journal, Filter{ThreadID: "parent"}, 10, "parent", "reply")
		entries, _, err := journal.Entries()
		want := []Entry{tx.Messages[0], tx.Messages[2], tx.Messages[3], tx.Messages[4]}
		if err != nil || !reflect.DeepEqual(entries, want) {
			t.Fatal("retention changed surviving admission facts", err)
		}
		for _, request := range tx.Requests {
			if request.MessageID != "dropped" {
				assertRequestRecovered(t, journal, request)
			}
		}
		_, found, err := journal.LookupRequest("sender-chain", "request-dropped")
		if err != nil || found {
			t.Fatal("deleted message retained a dangling alias", err)
		}
	}
	// The strict cutoff retains boundary messages; a later startup can remove
	// that thread and the entire now-old reply thread, while keeping pending.
	if err := j.Compact(now.Add(31*24*time.Hour), DefaultRetention); err != nil {
		t.Fatal(err)
	}
	assertMatches(t, newTestJournal(t, testFiles{}, path), Filter{}, 10, "pending")
	mustCommit(t, j, sentTransaction("next", 11))
	assertMatches(t, newTestJournal(t, testFiles{}, path), Filter{}, 11, "pending", "next")
}

func TestJournalRetentionFailurePreservesBytesAndIndexes(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	files := &testFiles{}
	j := newTestJournal(t, files, path)
	tx, now := retentionFixture()
	mustCommit(t, j, tx)
	before := readBytes(t, path)
	files.replace = func(string, []byte) error { return errors.New("private host diagnostic") }
	if err := j.Compact(now, DefaultRetention); !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "private") {
		t.Fatal("replacement failure did not return a safe typed error", err)
	}
	if !bytes.Equal(before, readBytes(t, path)) {
		t.Fatal("failed replacement changed committed bytes")
	}
	for _, journal := range []*Journal{j, newTestJournal(t, testFiles{}, path)} {
		assertMatches(t, journal, Filter{}, 10, "parent", "dropped", "reply", "pending", "boundary")
		assertRequestRecovered(t, journal, tx.Requests[1])
	}
	files.replace = nil
	if err := j.Compact(now, DefaultRetention); err != nil {
		t.Fatal("ordinary replacement failure prevented a later retry", err)
	}
}

func TestJournalRetentionEmptySnapshotKeepsSequence(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	j := newTestJournal(t, testFiles{}, path)
	entry := message("old", 1)
	entry.Message.Status = agentmessages.Expired
	mustCommit(t, j, transaction(8, Snapshot, entry))
	if err := j.Compact(entry.Message.SentAt.Add(40*24*time.Hour), DefaultRetention); err != nil {
		t.Fatal(err)
	}
	rebuilt := newTestJournal(t, testFiles{}, path)
	assertMatches(t, rebuilt, Filter{}, 8)
	mustCommit(t, rebuilt, sentTransaction("new", 9))
	assertMatches(t, newTestJournal(t, testFiles{}, path), Filter{}, 9, "new")
}

func TestJournalRetentionInvalidOrUnneededMakesNoReplacement(t *testing.T) {
	t.Parallel()
	files := testFiles{replace: func(string, []byte) error {
		t.Error("unneeded compaction attempted a replacement")
		return ErrUnavailable
	}}
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	j := newTestJournal(t, files, path)
	now := message("example", 1).Message.SentAt
	if err := j.Compact(now, DefaultRetention); err != nil {
		t.Fatal("empty journal compaction failed", err)
	}
	for _, input := range []struct {
		now       time.Time
		retention time.Duration
	}{{time.Time{}, DefaultRetention}, {now, 0}, {now, time.Hour}} {
		if err := j.Compact(input.now, input.retention); !errors.Is(err, ErrInvalidTransaction) {
			t.Fatal("invalid retention accepted", err)
		}
	}
	mustCommit(t, j, sentTransaction("pending", 1))
	if err := j.Compact(now.Add(40*24*time.Hour), DefaultRetention); err != nil {
		t.Fatal("pending message triggered retention", err)
	}
}

func TestJournalRetentionRefusesCorruptOrUnopenedStore(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	files := testFiles{replace: func(string, []byte) error {
		t.Error("unavailable store attempted replacement")
		return nil
	}}
	now := message("example", 1).Message.SentAt
	j := New(files, path)
	if err := j.Compact(now, DefaultRetention); !errors.Is(err, ErrUnavailable) {
		t.Fatal("unopened store admitted compaction", err)
	}
	corrupt := []byte("private-invalid-record\n")
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := j.Open(); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	if err := j.Compact(now, DefaultRetention); !errors.Is(err, ErrCorrupt) {
		t.Fatal("compaction repaired corrupt bytes", err)
	}
	if !bytes.Equal(corrupt, readBytes(t, path)) {
		t.Fatal("compaction modified corrupt store")
	}
}

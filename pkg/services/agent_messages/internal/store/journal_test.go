package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

type testFiles struct {
	open func(string, int, fs.FileMode) (io.WriteCloser, error)
}

func (testFiles) ReadFile(path string) ([]byte, error)         { return os.ReadFile(path) }
func (testFiles) MkdirAll(path string, mode fs.FileMode) error { return os.MkdirAll(path, mode) }
func (files testFiles) OpenFile(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
	if files.open != nil {
		return files.open(path, flags, mode)
	}
	return os.OpenFile(path, flags, mode)
}

func transaction(sequence uint64, kind string, entries ...Entry) Transaction {
	return Transaction{Version: Version, RecordID: kind + "-" + time.Unix(int64(sequence), 0).Format(time.RFC3339),
		Sequence: sequence, Kind: kind, CommittedAt: time.Date(2026, 10, 11, 1, 0, 0, 0, time.UTC),
		Messages: entries, Requests: []Request{}}
}

func message(id string, sequence uint64) Entry {
	text := "safe message " + id
	digest := sha256.Sum256([]byte(text))
	sent := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	return Entry{SenderChainIdentity: "sender-chain", RecipientChainIdentity: "recipient-chain", Sequence: sequence,
		Message: agentmessages.Message{MessageID: id, ThreadID: id,
			From: agentmessages.Sender{Principal: "WORKER", WorkerSessionID: "sender"},
			To:   agentmessages.Recipient{Kind: "WORKER_SESSION", WorkerSessionID: "recipient"},
			Body: text, BodySHA256: hex.EncodeToString(digest[:]), Delivery: "QUEUE", IfEnded: "REVIVE", ReplyIfEnded: "HOLD",
			Status: agentmessages.Queued, SentAt: sent, ExpiresAt: sent.Add(24 * time.Hour)}}
}

func sentTransaction(id string, sequence uint64) Transaction {
	t := transaction(sequence, Sent, message(id, sequence))
	t.Requests = []Request{{SenderIdentity: "sender-chain", RequestID: "request-" + id,
		RequestSHA256: strings.Repeat("a", 64), MessageID: id}}
	return t
}

func newTestJournal(t *testing.T, files FileSystem, path string) *Journal {
	t.Helper()
	j := New(files, path)
	if err := j.Open(); err != nil {
		t.Fatal(err)
	}
	return j
}

func mustCommit(t *testing.T, journal *Journal, tx Transaction) {
	t.Helper()
	if err := journal.Commit(tx); err != nil {
		t.Fatal(err)
	}
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertRequestRecovered(t *testing.T, journal *Journal, request Request) {
	t.Helper()
	got, exists, err := journal.LookupRequest(request.SenderIdentity, request.RequestID)
	if err != nil || !exists || got != request {
		t.Fatalf("request alias was not recovered: %v", err)
	}
}

// The store component is the subject: real files prove its commit/reconstruction
// contract, while short writes and flush failures are controlled descriptors.
func TestJournalPreservesMultibyteRequestIDAtCharacterLimit(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	journal := New(testFiles{}, path)
	if err := journal.Open(); err != nil {
		t.Fatal(err)
	}
	original := sentTransaction("first", 1)
	original.Requests[0].RequestID = strings.Repeat("é", 200)
	if err := journal.Commit(original); err != nil {
		t.Fatalf("200-character request rejected: %v", err)
	}
	reconstructed := New(testFiles{}, path)
	if err := reconstructed.Open(); err != nil {
		t.Fatal(err)
	}
	request := original.Requests[0]
	got, exists, err := reconstructed.LookupRequest(request.SenderIdentity, request.RequestID)
	if err != nil || !exists || got != request {
		t.Fatalf("multibyte request did not survive reconstruction: %v", err)
	}
	alias := transaction(2, RequestAlias)
	alias.Messages = []Entry{}
	alias.Requests = []Request{request}
	alias.Requests[0].RequestID += "é"
	if err := journal.Commit(alias); !errors.Is(err, ErrInvalidTransaction) {
		t.Fatalf("201-character request accepted: %v", err)
	}
}

func TestJournalReconstructsConversationAndRequestAlias(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	j := newTestJournal(t, testFiles{}, path)
	original := sentTransaction("msg-root", 1)
	original.Messages[0].Message.Body = "[REDACTED]"
	digest := sha256.Sum256([]byte(original.Messages[0].Message.Body))
	original.Messages[0].Message.BodySHA256 = hex.EncodeToString(digest[:])
	original.Messages[0].Message.BodyRedactionCount = 1
	mustCommit(t, j, original)
	parent := original.Messages[0]
	parent.Message.Status = agentmessages.Read
	mustCommit(t, j, transaction(2, Read, parent))
	reply := message("msg-reply", 3)
	reply.Message.ThreadID, reply.Message.InReplyTo = parent.Message.ThreadID, parent.Message.MessageID
	reply.Message.From.WorkerSessionID, reply.Message.To.WorkerSessionID = "recipient", "sender"
	reply.SenderChainIdentity, reply.RecipientChainIdentity = "recipient-chain", "sender-chain"
	parent.Message.Status, parent.Message.RepliedByMessageID = agentmessages.Replied, reply.Message.MessageID
	answer := transaction(3, Replied, parent, reply)
	answer.Requests = []Request{{SenderIdentity: reply.SenderChainIdentity, RequestID: "reply-key",
		RequestSHA256: strings.Repeat("b", 64), MessageID: reply.Message.MessageID}}
	mustCommit(t, j, answer)
	alias := transaction(4, RequestAlias)
	alias.Messages = []Entry{}
	alias.Requests = []Request{original.Requests[0]}
	alias.Requests[0].RequestID = "alias-key"
	mustCommit(t, j, alias)
	before, seq, err := j.Entries()
	if err != nil || seq != 4 || len(before) != 2 {
		t.Fatalf("entries=%v sequence=%d error=%v", before, seq, err)
	}
	restored := newTestJournal(t, testFiles{}, path)
	after, restoredSeq, err := restored.Entries()
	if err != nil || restoredSeq != seq || !reflect.DeepEqual(before, after) {
		t.Fatalf("conversation changed across reconstruction: %v", err)
	}
	for _, request := range []Request{original.Requests[0], answer.Requests[0], alias.Requests[0]} {
		assertRequestRecovered(t, restored, request)
	}
	if after[0].Message.Status != agentmessages.Replied || after[0].Message.RepliedByMessageID != "msg-reply" ||
		after[1].Message.InReplyTo != "msg-root" || after[1].Message.BodySHA256 != reply.Message.BodySHA256 {
		t.Fatal("lost reply or content identity")
	}
	regressed := after[0]
	regressed.Message.Status = agentmessages.Read
	if err := restored.Commit(transaction(5, Read, regressed)); !errors.Is(err, ErrInvalidTransaction) {
		t.Fatal("replied message regressed to READ", err)
	}
	// Returned values cannot mutate the store's authoritative index.
	after[0].Message.Body = "changed"
	fresh, _, err := restored.Entries()
	if err != nil || fresh[0].Message.Body != "[REDACTED]" {
		t.Fatal("read result aliases canonical state")
	}
}

type faultFile struct {
	*os.File
	fault     string
	syncCalls int
}

func (file *faultFile) Write(data []byte) (int, error) {
	switch file.fault {
	case "short":
		return file.File.Write(data[:len(data)/2])
	case "write":
		written, _ := file.File.Write(data[:len(data)/2])
		return written, errors.New("planted secret in write error")
	default:
		return file.File.Write(data)
	}
}

func (file *faultFile) Sync() error {
	file.syncCalls++
	if (file.fault == "sync" || file.fault == "rollback-truncate") && file.syncCalls == 1 {
		return errors.New("planted secret in flush error")
	}
	if file.fault == "rollback-sync" {
		return errors.New("flush unavailable")
	}
	return file.File.Sync()
}

func (file *faultFile) Truncate(size int64) error {
	if file.fault == "rollback-truncate" {
		return errors.New("truncate unavailable")
	}
	return file.File.Truncate(size)
}

func (file *faultFile) Close() error {
	err := file.File.Close()
	if file.fault == "close" {
		return errors.New("close unavailable after flush")
	}
	return err
}

func TestJournalRollsBackWriteAndFlushFailures(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"short", "write", "sync", "open"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "messages.jsonl")
			files := &testFiles{}
			j := newTestJournal(t, files, path)
			original := sentTransaction("msg-root", 1)
			if err := j.Commit(original); err != nil {
				t.Fatal(err)
			}
			before := readBytes(t, path)
			files.open = func(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
				if fault == "open" {
					return nil, errors.New("planted secret in open error")
				}
				file, err := os.OpenFile(path, flags, mode)
				if err != nil {
					return nil, err
				}
				return &faultFile{File: file, fault: fault}, nil
			}
			next := original.Messages[0]
			next.Message.Status = agentmessages.Read
			if err := j.Commit(transaction(2, Read, next)); !errors.Is(err, ErrUnavailable) ||
				strings.Contains(err.Error(), "planted secret") {
				t.Fatalf("unsafe/untyped error: %v", err)
			}
			after := readBytes(t, path)
			if string(before) != string(after) {
				t.Fatal("failed write changed committed bytes")
			}
			entries, seq, err := j.Entries()
			if err != nil || seq != 1 || !reflect.DeepEqual(entries, original.Messages) {
				t.Fatal("failed write changed index")
			}
			files.open = nil
			if err := j.Commit(transaction(2, Read, next)); err != nil {
				t.Fatal("failed write spent the next sequence", err)
			}
			restored := newTestJournal(t, files, path)
			entries, seq, err = restored.Entries()
			if err != nil || seq != 2 || entries[0].Message.Status != agentmessages.Read {
				t.Fatal("retry was not durably recovered")
			}
		})
	}
}

func TestJournalFlushCommitsDespiteLateCloseError(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	files := testFiles{open: func(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
		file, err := os.OpenFile(path, flags, mode)
		if err != nil {
			return nil, err
		}
		return &faultFile{File: file, fault: "close"}, nil
	}}
	j := newTestJournal(t, files, path)
	if err := j.Commit(sentTransaction("msg-root", 1)); err != nil {
		t.Fatal(err)
	}
	restored := newTestJournal(t, testFiles{}, path)
	entries, seq, err := restored.Entries()
	if err != nil || len(entries) != 1 || seq != 1 {
		t.Fatal("flushed commit was not recovered")
	}
}

func TestJournalCorruptionPreservesOriginalBytesAndFailsClosed(t *testing.T) {
	t.Parallel()
	valid := sentTransaction("msg-root", 1)
	encoded, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	validLine := string(encoded) + "\n"
	badHash := valid
	badHash.Messages = append([]Entry{}, valid.Messages...)
	badHash.Messages[0].Message.BodySHA256 = strings.Repeat("c", 64)
	badEncoded, err := json.Marshal(badHash)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"invalid UTF-8":      strings.Replace(validLine, "safe message", string([]byte{0xff}), 1),
		"partial tail":       validLine + "{\"version\":1",
		"missing newline":    string(encoded),
		"unknown field":      strings.TrimSuffix(string(encoded), "}") + ",\"token\":\"secret\"}\n",
		"invalid version":    strings.Replace(validLine, "\"version\":1", "\"version\":2", 1),
		"duplicate sequence": validLine + validLine,
		"unsafe hash":        string(badEncoded) + "\n",
		"extra json value":   string(encoded) + " {}\n",
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "messages.jsonl")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			j := New(testFiles{}, path)
			if err := j.Open(); !errors.Is(err, ErrCorrupt) {
				t.Fatalf("expected typed corruption: %v", err)
			}
			if _, _, err := j.Entries(); !errors.Is(err, ErrCorrupt) {
				t.Fatal("corruption returned partial index", err)
			}
			if err := j.Commit(sentTransaction("msg-new", 2)); !errors.Is(err, ErrCorrupt) {
				t.Fatal("corruption admitted write", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != contents {
				t.Fatal("corruption recovery modified original bytes")
			}
		})
	}
}

func TestJournalRejectsNonatomicReplyAndRequestConflicts(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	j := newTestJournal(t, testFiles{}, path)
	original := sentTransaction("msg-root", 1)
	if err := j.Commit(original); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parent := original.Messages[0]
	parent.Message.Status, parent.Message.RepliedByMessageID = agentmessages.Replied, "msg-reply"
	missingChild := transaction(2, Replied, parent)
	alias := transaction(2, RequestAlias)
	alias.Messages, alias.Requests = []Entry{}, []Request{original.Requests[0]}
	alias.Requests[0].RequestSHA256 = strings.Repeat("b", 64)
	missingTarget := alias
	missingTarget.Requests = append([]Request{}, alias.Requests...)
	missingTarget.Requests[0].RequestID, missingTarget.Requests[0].MessageID = "new-key", "missing"
	for _, change := range []Transaction{missingChild, alias, missingTarget} {
		if err := j.Commit(change); !errors.Is(err, ErrInvalidTransaction) {
			t.Fatal("invalid admission accepted", err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("invalid admission changed bytes")
	}
}

func TestJournalExpiredAndRepliedNeverRegressToRead(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	j := newTestJournal(t, testFiles{}, path)
	first := sentTransaction("msg-root", 1)
	if err := j.Commit(first); err != nil {
		t.Fatal(err)
	}
	expired := first.Messages[0]
	expired.Message.Status = agentmessages.Expired
	if err := j.Commit(transaction(2, Expired, expired)); err != nil {
		t.Fatal(err)
	}
	regressed := expired
	regressed.Message.Status = agentmessages.Read
	if err := j.Commit(transaction(3, Read, regressed)); !errors.Is(err, ErrInvalidTransaction) {
		t.Fatal(err)
	}
	restored := newTestJournal(t, testFiles{}, path)
	entries, _, err := restored.Entries()
	if err != nil || entries[0].Message.Status != agentmessages.Expired {
		t.Fatal("expiry not persisted")
	}
}

func TestJournalConcurrentSameSequenceCommitsOnce(t *testing.T) {
	t.Parallel()
	j := newTestJournal(t, testFiles{}, filepath.Join(t.TempDir(), "messages.jsonl"))
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, id := range []string{"msg-a", "msg-b"} {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			results <- j.Commit(sentTransaction(id, 1))
		}()
	}
	close(start)
	group.Wait()
	close(results)
	successes, refusals := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrInvalidTransaction) {
			refusals++
		} else {
			t.Fatal(err)
		}
	}
	entries, seq, err := j.Entries()
	if err != nil || successes != 1 || refusals != 1 || seq != 1 || len(entries) != 1 {
		t.Fatal("concurrent admission was not serialized")
	}
}

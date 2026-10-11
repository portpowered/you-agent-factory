package store

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

func TestJournalRejectsInvalidContentBeforeOpeningWriter(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*Transaction){
		"blank body":              func(tx *Transaction) { tx.Messages[0].Message.Body = " \n" },
		"invalid UTF-8":           func(tx *Transaction) { tx.Messages[0].Message.Body = string([]byte{0xff}) },
		"oversized multibyte":     func(tx *Transaction) { tx.Messages[0].Message.Body = strings.Repeat("é", 4097) },
		"wrong safe hash":         func(tx *Transaction) { tx.Messages[0].Message.BodySHA256 = strings.Repeat("b", 64) },
		"secret redaction count":  func(tx *Transaction) { tx.Messages[0].Message.BodyRedactionCount = -1 },
		"operator sender":         func(tx *Transaction) { tx.Messages[0].Message.From.Principal = "OPERATOR" },
		"operator target":         func(tx *Transaction) { tx.Messages[0].Message.To.Kind = "OPERATOR" },
		"missing sender chain":    func(tx *Transaction) { tx.Messages[0].SenderChainIdentity = "" },
		"missing recipient chain": func(tx *Transaction) { tx.Messages[0].RecipientChainIdentity = "" },
		"self address":            func(tx *Transaction) { tx.Messages[0].Message.To.WorkerSessionID = "sender" },
		"interrupt":               func(tx *Transaction) { tx.Messages[0].Message.Delivery = "INTERRUPT" },
		"unknown ended policy":    func(tx *Transaction) { tx.Messages[0].Message.IfEnded = "UNKNOWN" },
		"unknown reply policy":    func(tx *Transaction) { tx.Messages[0].Message.ReplyIfEnded = "UNKNOWN" },
		"unknown status":          func(tx *Transaction) { tx.Messages[0].Message.Status = "UNKNOWN" },
		"revive delivered":        func(tx *Transaction) { tx.Messages[0].Message.RevivedWorkerSessionID = "new" },
		"unsafe reason":           func(tx *Transaction) { tx.Messages[0].Message.Reason = "sensitive diagnostic" },
		"too short expiry": func(tx *Transaction) {
			tx.Messages[0].Message.ExpiresAt = tx.Messages[0].Message.SentAt.Add(59 * time.Second)
		},
		"too long expiry": func(tx *Transaction) {
			tx.Messages[0].Message.ExpiresAt = tx.Messages[0].Message.SentAt.Add(7*24*time.Hour + time.Second)
		},
		"hop overflow":           func(tx *Transaction) { tx.Messages[0].Message.Hop = 4 },
		"oversized request":      func(tx *Transaction) { tx.Requests[0].RequestID = strings.Repeat("x", 201) },
		"foreign request sender": func(tx *Transaction) { tx.Requests[0].SenderIdentity = "unrelated" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opened := false
			files := testFiles{open: func(string, int, fs.FileMode) (io.WriteCloser, error) {
				opened = true
				return nil, errors.New("writer should not be opened")
			}}
			j := newTestJournal(t, files, filepath.Join(t.TempDir(), "messages.jsonl"))
			tx := sentTransaction("msg-root", 1)
			change(&tx)
			if err := j.Commit(tx); !errors.Is(err, ErrInvalidTransaction) {
				t.Fatal("malformed record accepted", err)
			}
			if opened {
				t.Fatal("invalid transaction reached the file effect")
			}
		})
	}
}

func TestJournalRollbackUncertaintyStopsFurtherWrites(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"rollback-sync", "rollback-truncate"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "messages.jsonl")
			files := &testFiles{}
			j := newTestJournal(t, files, path)
			if err := j.Commit(sentTransaction("msg-root", 1)); err != nil {
				t.Fatal(err)
			}
			files.open = func(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
				file, err := os.OpenFile(path, flags, mode)
				if err != nil {
					return nil, err
				}
				return &faultFile{File: file, fault: fault}, nil
			}
			if err := j.Commit(sentTransaction("msg-next", 2)); !errors.Is(err, ErrUnavailable) {
				t.Fatal(err)
			}
			afterFailure, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			files.open = nil
			if err := j.Commit(sentTransaction("msg-another", 2)); !errors.Is(err, ErrUnavailable) {
				t.Fatal("uncertain tail admitted another write", err)
			}
			afterRetry, err := os.ReadFile(path)
			if err != nil || string(afterRetry) != string(afterFailure) {
				t.Fatal("uncertain journal bytes changed")
			}
			if _, _, err := j.Entries(); !errors.Is(err, ErrUnavailable) {
				t.Fatal("uncertain journal published success", err)
			}
		})
	}
}

func TestJournalReadCannotAlterBodyOrVerifiedIdentities(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"body", "recipient-chain", "sender-work", "sequence"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			j := newTestJournal(t, testFiles{}, filepath.Join(t.TempDir(), "messages.jsonl"))
			original := sentTransaction("msg-root", 1)
			if err := j.Commit(original); err != nil {
				t.Fatal(err)
			}
			next := original.Messages[0]
			next.Message.Status = agentmessages.Read
			switch field {
			case "body":
				next.Message.Body = "modified content"
			case "recipient-chain":
				next.RecipientChainIdentity = "unrelated"
			case "sender-work":
				next.SenderWorkIdentity = "invented work"
			case "sequence":
				next.Sequence = 2
			}
			if err := j.Commit(transaction(2, Read, next)); !errors.Is(err, ErrInvalidTransaction) {
				t.Fatal("READ changed immutable admission facts", err)
			}
			entries, seq, err := j.Entries()
			if err != nil || seq != 1 || entries[0].Message.Status != agentmessages.Queued {
				t.Fatal("refused READ changed message")
			}
		})
	}
}

func TestJournalSnapshotRetainsConversationAndAliases(t *testing.T) {
	t.Parallel()
	j := newTestJournal(t, testFiles{}, filepath.Join(t.TempDir(), "messages.jsonl"))
	first := sentTransaction("msg-root", 17)
	snapshot := transaction(18, Snapshot, first.Messages[0])
	snapshot.Requests = first.Requests
	if err := j.Commit(snapshot); err != nil {
		t.Fatal(err)
	}
	got, exists, err := j.LookupRequest(first.Requests[0].SenderIdentity, first.Requests[0].RequestID)
	if err != nil || !exists || got != first.Requests[0] {
		t.Fatal("snapshot lost alias")
	}
	if err := j.Commit(transaction(19, Snapshot, first.Messages[0])); !errors.Is(err, ErrInvalidTransaction) {
		t.Fatal("snapshot replaced live index", err)
	}
}

func TestJournalAdmissionExpirySurvivesReconstruction(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{Sent, RequestAlias} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "messages.jsonl")
			j := newTestJournal(t, testFiles{}, path)
			original := sentTransaction("msg-root", 1)
			mustCommit(t, j, original)
			expired := original.Messages[0]
			expired.Message.Status = agentmessages.Expired
			tx := sentTransaction("msg-next", 2)
			if kind == RequestAlias {
				tx.Kind = RequestAlias
				tx.Messages = []Entry{}
				tx.Requests[0].MessageID = "msg-root"
			}
			tx.CommittedAt = expired.Message.ExpiresAt
			tx.Messages = append(tx.Messages, expired)
			mustCommit(t, j, tx)
			reconstructed := newTestJournal(t, testFiles{}, path)
			entries, seq, err := reconstructed.Entries()
			if err != nil || seq != 2 || entries[0].Message.Status != agentmessages.Expired {
				t.Fatal("admission expiry lost on restart", err)
			}
			assertRequestRecovered(t, reconstructed, tx.Requests[0])
		})
	}
}

func TestJournalAdmissionRefusesInvalidExpiryWithoutChangingBytes(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"early", "immutable", "unknown", "alias-read"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "messages.jsonl")
			j := newTestJournal(t, testFiles{}, path)
			original := sentTransaction("msg-root", 1)
			mustCommit(t, j, original)
			before := readBytes(t, path)
			expired := original.Messages[0]
			expired.Message.Status = agentmessages.Expired
			tx := sentTransaction("msg-next", 2)
			tx.CommittedAt = expired.Message.ExpiresAt
			switch fault {
			case "early":
				tx.CommittedAt = tx.CommittedAt.Add(-time.Nanosecond)
			case "immutable":
				expired.RecipientChainIdentity = "foreign"
			case "unknown":
				expired.Message.MessageID = "missing"
			case "alias-read":
				tx.Kind = RequestAlias
				tx.Messages = []Entry{}
				tx.Requests[0].MessageID = "msg-root"
				expired.Message.Status = agentmessages.Read
			}
			tx.Messages = append(tx.Messages, expired)
			if err := j.Commit(tx); !errors.Is(err, ErrInvalidTransaction) {
				t.Fatal("invalid admission expiry accepted", err)
			}
			if string(readBytes(t, path)) != string(before) {
				t.Fatal("invalid expiry changed bytes")
			}
		})
	}
}

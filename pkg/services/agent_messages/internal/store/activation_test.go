package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/filesystem"
	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

func TestActivationUsesDurableHostReplacementAndReconstructs(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages", "journal.jsonl")
	files := filesystem.Local{}
	var _ agentmessages.StoreFileSystem = files
	j := New(files, path)
	tx, now := retentionFixture()
	if err := j.Activate(now, DefaultRetention); err != nil {
		t.Fatal(err)
	}
	mustCommit(t, j, tx)
	rebuilt := New(files, path)
	if err := rebuilt.Activate(now, DefaultRetention); err != nil {
		t.Fatal(err)
	}
	assertMatches(t, rebuilt, Filter{}, 10, "parent", "reply", "pending", "boundary")
	assertRequestRecovered(t, rebuilt, tx.Requests[len(tx.Requests)-1])
	mustCommit(t, rebuilt, sentTransaction("next", 11))
	last := New(files, path)
	if err := last.Activate(now, DefaultRetention); err != nil {
		t.Fatal(err)
	}
	assertMatches(t, last, Filter{}, 11, "parent", "reply", "pending", "boundary", "next")
}

func TestActivationFailureQuarantinesOnlyItsStore(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	seed := newTestJournal(t, testFiles{}, path)
	tx, now := retentionFixture()
	mustCommit(t, seed, tx)
	before := readBytes(t, path)
	files := &testFiles{replace: func(string, []byte) error { return errors.New("private host diagnostic") }}
	j := New(files, path)
	if err := j.Activate(now, DefaultRetention); !errors.Is(err, agentmessages.ErrStoreUnavailable) {
		t.Fatal("activation must return the public safe store error", err)
	}
	if _, _, err := j.Entries(); !errors.Is(err, agentmessages.ErrStoreUnavailable) {
		t.Fatal("failed activation exposed reconstructed records", err)
	}
	if err := j.Commit(sentTransaction("denied", 11)); !errors.Is(err, agentmessages.ErrStoreUnavailable) {
		t.Fatal("failed activation admitted another write", err)
	}
	if !bytes.Equal(before, readBytes(t, path)) {
		t.Fatal("failed activation changed the durable journal")
	}
	// A new process can recover; the old instance stays unavailable even when
	// the replace effect recovers, rather than bypassing activation readiness.
	files.replace = nil
	if err := j.Open(); !errors.Is(err, agentmessages.ErrStoreUnavailable) {
		t.Fatal("Open bypassed failed activation", err)
	}
	recovered := New(files, path)
	if err := recovered.Activate(now, DefaultRetention); err != nil {
		t.Fatal(err)
	}
	assertMatches(t, recovered, Filter{}, 10, "parent", "reply", "pending", "boundary")
}

func TestActivationCorruptionPreservesBytesAndPublicError(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	before := []byte("private malformed journal\n")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	j := New(filesystem.Local{}, path)
	if err := j.Activate(time.Now(), DefaultRetention); !errors.Is(err, agentmessages.ErrStoreCorrupt) {
		t.Fatal("corruption must remain a safe public store error", err)
	}
	if !bytes.Equal(before, readBytes(t, path)) {
		t.Fatal("activation repaired corrupt bytes")
	}
	if err := j.Commit(sentTransaction("denied", 1)); !errors.Is(err, agentmessages.ErrStoreCorrupt) {
		t.Fatal("corrupt store admitted a write", err)
	}
}

func TestActivationRejectsInvalidSettingsBeforeFileEffects(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		now       time.Time
		retention time.Duration
	}{
		{"missing time", time.Time{}, DefaultRetention},
		{"missing retention", time.Now(), 0},
		{"short retention", time.Now(), 24*time.Hour - time.Nanosecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// A nil filesystem proves invalid activation never acquires an
			// effect or exposes a writable journal after the reported failure.
			j := New(nil, "unused")
			if err := j.Activate(tc.now, tc.retention); !errors.Is(err, ErrInvalidTransaction) {
				t.Fatal("invalid settings were accepted", err)
			}
			if _, _, err := j.Entries(); !errors.Is(err, ErrInvalidTransaction) {
				t.Fatal("failed activation did not preserve its error", err)
			}
			if err := j.Open(); !errors.Is(err, ErrInvalidTransaction) {
				t.Fatal("Open bypassed invalid activation", err)
			}
		})
	}
}

func TestActivationKeepsReadsBehindRetentionPublication(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	tx, now := retentionFixture()
	mustCommit(t, newTestJournal(t, testFiles{}, path), tx)
	entered, release := make(chan struct{}), make(chan struct{})
	files := testFiles{replace: func(path string, data []byte) error {
		close(entered)
		<-release
		return os.WriteFile(path, data, 0o600)
	}}
	j := New(files, path)
	activated := make(chan error, 1)
	go func() { activated <- j.Activate(now, DefaultRetention) }()
	<-entered
	readStarted := make(chan struct{})
	readDone := make(chan []Entry, 1)
	readErr := make(chan error, 1)
	go func() {
		close(readStarted)
		entries, _, err := j.Entries()
		readDone <- entries
		readErr <- err
	}()
	<-readStarted
	select {
	case <-readDone:
		t.Error("read escaped before activation published retention")
	default:
	}
	close(release)
	if err := <-activated; err != nil {
		t.Fatal(err)
	}
	entries := <-readDone
	if err := <-readErr; err != nil || len(entries) != 4 {
		t.Fatal("read did not observe the retained snapshot", err)
	}
	for _, entry := range entries {
		if entry.Message.MessageID == "dropped" {
			t.Fatal("read exposed a record deleted during activation")
		}
	}
}

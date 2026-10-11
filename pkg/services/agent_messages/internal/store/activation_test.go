package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
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

func TestDeferredProfileSelectionReconstructsWithoutCrossingProfiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "selected", "messages.jsonl")
	other := filepath.Join(root, "other", "messages.jsonl")
	files := filesystem.Local{}
	tx, now := retentionFixture()
	mustCommit(t, newTestJournal(t, files, path), tx)
	mustCommit(t, newTestJournal(t, files, other), sentTransaction("other-profile", 1))
	j := New(files, "")
	if err := j.Open(); !errors.Is(err, ErrUnavailable) {
		t.Fatal("unbound journal discovered a path", err)
	}
	for _, invalid := range []string{"", "relative/messages.jsonl"} {
		if err := j.Bind(invalid); !errors.Is(err, agentmessages.ErrBadRequest) {
			t.Fatal("journal accepted an implicit profile", err)
		}
	}
	if err := j.Bind(path); err != nil {
		t.Fatal(err)
	}
	if err := j.Activate(now, DefaultRetention); err != nil {
		t.Fatal(err)
	}
	assertMatches(t, j, Filter{}, 10, "parent", "reply", "pending", "boundary")
	assertRequestRecovered(t, j, tx.Requests[len(tx.Requests)-1])
	if err := j.Bind(other); !errors.Is(err, agentmessages.ErrBadRequest) {
		t.Fatal("active journal switched profiles", err)
	}
	if err := j.Bind(filepath.Join(root, "selected", ".", "messages.jsonl")); err != nil {
		t.Fatal("same canonical profile was rejected", err)
	}
	mustCommit(t, j, sentTransaction("next", 11))
	rebuilt := newTestJournal(t, files, path)
	assertMatches(t, rebuilt, Filter{}, 11, "parent", "reply", "pending", "boundary", "next")
	assertMatches(t, newTestJournal(t, files, other), Filter{}, 1, "other-profile")
}

func TestFailedProfileRemainsSelected(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "corrupt.jsonl")
	before := []byte("private malformed journal\n")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	j := New(filesystem.Local{}, "")
	if err := j.Bind(path); err != nil {
		t.Fatal(err)
	}
	if err := j.Activate(time.Unix(1, 0), DefaultRetention); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	if err := j.Bind(filepath.Join(root, "empty.jsonl")); !errors.Is(err, agentmessages.ErrBadRequest) {
		t.Fatal("corruption was bypassed by switching profiles", err)
	}
	if _, _, err := j.Entries(); !errors.Is(err, ErrCorrupt) || !bytes.Equal(before, readBytes(t, path)) {
		t.Fatal("rejected selection changed quarantined bytes or readiness", err)
	}
}

func TestConcurrentProfileSelectionAdmitsExactlyOneIdentity(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	paths := []string{filepath.Join(root, "first.jsonl"), filepath.Join(root, "second.jsonl")}
	// Binding has no filesystem effects. Concurrent selectors compete for one
	// identity before any activation, rather than replacing each other's path.
	j := New(nil, "")
	results := make(chan string, len(paths))
	var callers sync.WaitGroup
	for _, path := range paths {
		callers.Add(1)
		go func() {
			defer callers.Done()
			if err := j.Bind(path); err == nil {
				results <- path
			} else if !errors.Is(err, agentmessages.ErrBadRequest) {
				t.Errorf("unexpected selection error: %v", err)
			}
		}()
	}
	callers.Wait()
	close(results)
	winners := []string{}
	for path := range results {
		winners = append(winners, path)
	}
	if len(winners) != 1 {
		t.Fatalf("accepted %d profile identities", len(winners))
	}
	if err := j.Bind(winners[0]); err != nil {
		t.Fatal("winning identity was not retained", err)
	}
	if _, _, err := j.Entries(); !errors.Is(err, ErrUnavailable) {
		t.Fatal("binding exposed an unactivated journal", err)
	}
}

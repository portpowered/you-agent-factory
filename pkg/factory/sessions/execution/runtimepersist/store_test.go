package runtimepersist_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
)

func TestNewProjectStore_ConstructsSnapshotBoundaryAndRoundTrips(t *testing.T) {
	projectRoot := t.TempDir()
	store, err := runtimepersist.NewProjectStore(projectRoot)
	if err != nil {
		t.Fatalf("NewProjectStore: %v", err)
	}
	if got, want := store.(runtimepersist.DirectoryStore).Dir, runtimepersist.DirForProjectRoot(projectRoot); got != want {
		t.Fatalf("store directory = %q, want %q", got, want)
	}
	sessionID := "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	payload := []byte(`{"status":"COMPLETED"}`)
	if err := store.Save(sessionID, payload); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := store.Load(sessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(loaded) != string(payload) {
		t.Fatalf("loaded payload = %s, want %s", loaded, payload)
	}
}

func TestNewProjectStore_RejectsMissingAndUnavailableRoots(t *testing.T) {
	if _, err := runtimepersist.NewProjectStore("   "); err == nil {
		t.Fatal("NewProjectStore(blank) error = nil")
	}
	blockedRoot := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedRoot, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := runtimepersist.NewProjectStore(blockedRoot); err == nil || !strings.Contains(err.Error(), "initialize durable session persistence directory") {
		t.Fatalf("NewProjectStore(blocked) error = %v, want actionable initialization failure", err)
	}
}

func TestSaveLoadBytes_RoundTripsSnapshotPayload(t *testing.T) {
	dir := t.TempDir()
	sessionID := "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := map[string]string{"sessionId": sessionID}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if err := runtimepersist.SaveBytes(dir, sessionID, encoded); err != nil {
		t.Fatalf("SaveBytes: %v", err)
	}
	loaded, err := runtimepersist.LoadBytes(dir, sessionID)
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if string(loaded) != string(encoded) {
		t.Fatalf("loaded payload = %s, want %s", loaded, encoded)
	}
	if _, err := os.Stat(filepath.Join(dir, sessionID+".json")); err != nil {
		t.Fatalf("stat persisted snapshot: %v", err)
	}
}

func TestSaveLoadBytes_AcceptsCanonicalFactorySessionIdentifiers(t *testing.T) {
	for _, sessionID := range []string{
		"~default",
		"12345678-1234-1234-1234-1234567890ab",
	} {
		t.Run(sessionID, func(t *testing.T) {
			dir := t.TempDir()
			if err := runtimepersist.SaveBytes(dir, sessionID, []byte(`{"status":"RUNNING"}`)); err != nil {
				t.Fatalf("SaveBytes: %v", err)
			}
			if _, err := runtimepersist.LoadBytes(dir, sessionID); err != nil {
				t.Fatalf("LoadBytes: %v", err)
			}
		})
	}
}

func TestSaveBytes_RejectsUnsafeSessionIdentifiers(t *testing.T) {
	for _, sessionID := range []string{"../escape", "session/child", "arbitrary"} {
		if err := runtimepersist.SaveBytes(t.TempDir(), sessionID, []byte(`{}`)); err == nil {
			t.Fatalf("SaveBytes(%q) succeeded", sessionID)
		}
	}
}

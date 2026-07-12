package runtimepersist_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/runtimepersist"
)

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

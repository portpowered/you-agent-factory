package api

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProviderSessionDetails_LoadsExactRolloutFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess_123", strings.Join([]string{
		`{"type":"session_meta","id":"sess_123"}`,
		`{"type":"response_item","item":{"type":"reasoning"}}`,
	}, "\n"))

	resp, err := loadProviderSessionDetails(root, "sess_123")
	if err != nil {
		t.Fatalf("loadProviderSessionDetails: %v", err)
	}
	if string(resp.ProviderSession.Provider) != "codex" || string(resp.ProviderSession.Kind) != "session_id" || resp.ProviderSession.Id != "sess_123" {
		t.Fatalf("provider session = %#v, want codex session_id sess_123", resp.ProviderSession)
	}
	if resp.Source.RelativePath != "2026/05/18/rollout-sess_123.jsonl" || resp.Source.SizeBytes == 0 {
		t.Fatalf("source = %#v, want rooted rollout metadata", resp.Source)
	}
	if resp.Parse.EventCount != 2 {
		t.Fatalf("parse summary = %#v, want two parsed events", resp.Parse)
	}
}

func TestLoadProviderSessionDetails_LoadsTimestampPrefixedRolloutFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	sessionID := "019e44f4-580e-7f32-981e-1e54ec6907d6"
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-"+sessionID+".jsonl", strings.Join([]string{
		`{"type":"session_meta","id":"` + sessionID + `"}`,
		`{"type":"response_item","payload":{"type":"reasoning"}}`,
	}, "\n"))

	resp, err := loadProviderSessionDetails(root, sessionID)
	if err != nil {
		t.Fatalf("loadProviderSessionDetails: %v", err)
	}
	wantRelativePath := "2026/05/18/rollout-2026-05-20T17-35-24-" + sessionID + ".jsonl"
	if resp.Source.RelativePath != wantRelativePath || resp.ProviderSession.Id != sessionID {
		t.Fatalf("provider session detail = %#v, want timestamp-prefixed rollout at %s", resp, wantRelativePath)
	}
}

func TestLoadProviderSessionDetails_PrefersExactRolloutWhenBothLayoutsExist(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess_123", `{"type":"session_meta","id":"sess_123"}`)
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)

	resp, err := loadProviderSessionDetails(root, "sess_123")
	if err != nil {
		t.Fatalf("loadProviderSessionDetails: %v", err)
	}
	if resp.Source.RelativePath != "2026/05/18/rollout-sess_123.jsonl" {
		t.Fatalf("relative path = %q, want exact rollout basename", resp.Source.RelativePath)
	}
}

func TestLoadProviderSessionDetails_NotFoundIsDistinguishable(t *testing.T) {
	_, err := loadProviderSessionDetails(t.TempDir(), "missing-session")
	if !errors.Is(err, errProviderSessionNotFound) {
		t.Fatalf("err = %v, want errProviderSessionNotFound", err)
	}
}

func TestLoadProviderSessionDetails_RejectsPathLikeIdentifiers(t *testing.T) {
	for _, id := range []string{"../secret", "/tmp/rollout-session.jsonl", "session.with.dot"} {
		t.Run(id, func(t *testing.T) {
			_, err := loadProviderSessionDetails(t.TempDir(), id)
			if !errors.Is(err, errInvalidProviderSessionIdentifier) {
				t.Fatalf("err = %v, want errInvalidProviderSessionIdentifier", err)
			}
		})
	}
}

func TestResolveCodexSessionFile_RejectsAmbiguousTimestampPrefixedMatches(t *testing.T) {
	root := t.TempDir()
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)
	sessionDir := filepath.Join(root, "2026", "05", "19")
	writeNamedProviderSessionFixtureAt(t, sessionDir, "rollout-2026-05-20T17-45-24-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)

	_, err := resolveCodexSessionFile(root, "sess_123")
	if !errors.Is(err, errAmbiguousProviderSessionFile) {
		t.Fatalf("err = %v, want errAmbiguousProviderSessionFile", err)
	}
}

func TestMatchesCodexSessionBaseName_AcceptsSupportedLayoutsOnly(t *testing.T) {
	exactName := "rollout-sess_123.jsonl"
	tests := []struct {
		baseName string
		want     bool
	}{
		{baseName: exactName, want: true},
		{baseName: "rollout-2026-05-20T17-35-24-sess_123.jsonl", want: true},
		{baseName: "rollout-backup-sess_123.jsonl", want: false},
		{baseName: "rollout-2026-05-20T17-35-24-backup-sess_123.jsonl", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.baseName, func(t *testing.T) {
			if got := matchesCodexSessionBaseName(tc.baseName, "sess_123", exactName); got != tc.want {
				t.Fatalf("matchesCodexSessionBaseName(%q) = %v, want %v", tc.baseName, got, tc.want)
			}
		})
	}
}

func writeNamedProviderSessionFixtureAt(t *testing.T, dir, fileName, contents string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create provider session fixture directory: %v", err)
	}
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write named provider session fixture: %v", err)
	}
}

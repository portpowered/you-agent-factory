package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

func TestLoadDetailsResolvesExactRollout(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "2026", "07", "16")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "rollout-session_123.jsonl"),
		[]byte("{\"type\":\"session_meta\"}\n"),
		0o600,
	); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	detail, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, "session_123")
	if err != nil {
		t.Fatalf("LoadDetails: %v", err)
	}
	if detail.ProviderSession.Provider != providersessions.ProviderCodex ||
		detail.ProviderSession.ID != "session_123" ||
		detail.Source.RelativePath != "2026/07/16/rollout-session_123.jsonl" {
		t.Fatalf("detail = %#v, want resolved codex rollout", detail)
	}
}

func TestParseDetailsKeepsMalformedAndUnknownDiagnostics(t *testing.T) {
	parsed, err := ParseDetails(strings.NewReader(strings.Join([]string{
		`{"type":"turn_context"}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}}`,
		`{"type":"future_event"}`,
		`{bad json`,
	}, "\n")))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	if parsed.Summary.EventCount != 3 ||
		parsed.Summary.MalformedLineCount != 1 ||
		parsed.Summary.UnknownEventCount != 1 ||
		len(parsed.Transcript) != 1 {
		t.Fatalf("parsed = %#v, want transcript plus malformed and unknown diagnostics", parsed)
	}
}

func TestLoadDetailsRejectsUnsafeIdentifier(t *testing.T) {
	_, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, t.TempDir(), "../session")
	if !errors.Is(err, providersessions.ErrInvalidIdentifier) {
		t.Fatalf("err = %v, want ErrInvalidIdentifier", err)
	}
}

package codex

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

func TestParseCodexSessionSummary_ExtractsDiagnosticDetails(t *testing.T) {
	session := strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"reasoning","summary":["checked input"],"encrypted_content":"sealed"}}`,
		`{"timestamp":"2026-05-18T10:00:02Z","type":"response_item","payload":{"type":"function_call","call_id":"call-1","name":"exec_command","arguments":{"cmd":"go test ./pkg/api"}}}`,
		`{"timestamp":"2026-05-18T10:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"ok"}}`,
		`{"timestamp":"2026-05-18T10:00:04Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":130}}}}`,
		`{"timestamp":"2026-05-18T10:00:05Z","type":"event_msg","payload":{"type":"new_future_event"}}`,
		`{"timestamp":"2026-05-18T10:00:06Z","type":"unexpected_top_level"}`,
		`{bad json`,
	}, "\n")

	parsed, err := ParseDetails(strings.NewReader(session))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	if parsed.Summary.MalformedLineCount != 1 || parsed.Summary.UnknownEventCount != 2 {
		t.Fatalf("summary = %#v, want malformed and unknown diagnostics", parsed.Summary)
	}
	if len(parsed.Summary.FunctionCalls) != 1 || len(parsed.Summary.Reasoning) != 1 {
		t.Fatalf("summary = %#v, want function-call and reasoning details", parsed.Summary)
	}
	if parsed.Summary.TokenUsage == nil || parsed.Summary.TokenUsage.TotalTokens == nil ||
		*parsed.Summary.TokenUsage.TotalTokens != 130 {
		t.Fatalf("token usage = %#v, want total 130", parsed.Summary.TokenUsage)
	}
}

func TestParseCodexSessionDetails_EmitsMixedTranscriptChronologically(t *testing.T) {
	session := strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Inspect the failing run."}]}}`,
		`{"timestamp":"2026-05-18T10:00:02Z","type":"response_item","payload":{"type":"reasoning","summary":["Checking tool output"]}}`,
		`{"timestamp":"2026-05-18T10:00:03Z","type":"response_item","payload":{"type":"function_call","call_id":"call-1","name":"exec_command","arguments":"go test"}}`,
		`{"timestamp":"2026-05-18T10:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"ok"}}`,
		`{"timestamp":"2026-05-18T10:00:05Z","type":"event_msg","payload":{"type":"agent_message","message":"The package tests passed."}}`,
	}, "\n")
	parsed, err := ParseDetails(strings.NewReader(session))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	want := []providersessions.TranscriptEntryType{
		providersessions.TranscriptUserMessage,
		providersessions.TranscriptReasoning,
		providersessions.TranscriptToolCall,
		providersessions.TranscriptToolOutput,
		providersessions.TranscriptAssistantMessage,
	}
	if len(parsed.Transcript) != len(want) {
		t.Fatalf("transcript = %#v, want %d entries", parsed.Transcript, len(want))
	}
	for index, wantType := range want {
		if parsed.Transcript[index].Type != wantType || parsed.Transcript[index].Order != index+1 {
			t.Fatalf("transcript[%d] = %#v, want type %q in order", index, parsed.Transcript[index], wantType)
		}
	}
}

func TestParseCodexSessionSummary_AcceptsLargeJSONLRecords(t *testing.T) {
	parsed, err := ParseDetails(strings.NewReader(
		`{"type":"response_item","payload":{"type":"reasoning","content":"` +
			strings.Repeat("x", 128*1024) + `"}}`,
	))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	if parsed.Summary.EventCount != 1 || len(parsed.Summary.Reasoning) != 1 {
		t.Fatalf("summary = %#v, want large reasoning record", parsed.Summary)
	}
}

func TestParseCodexSessionDetails_PreservesLongMessageContent(t *testing.T) {
	longPart := strings.Repeat("skill description ", 90) + "final-visible-tail"
	parsed, err := ParseDetails(strings.NewReader(
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"permissions block"},{"type":"input_text","text":"` +
			longPart + `"}]}}`,
	))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	if len(parsed.Transcript) != 1 || parsed.Transcript[0].Text == nil ||
		!strings.Contains(*parsed.Transcript[0].Text, "final-visible-tail") {
		t.Fatalf("transcript = %#v, want complete joined message", parsed.Transcript)
	}
}

func TestParseCodexSessionDetails_ReconcilesMirroredCodexMessages(t *testing.T) {
	message := "I will inspect the factory state first."
	parsed, err := ParseDetails(strings.NewReader(strings.Join([]string{
		`{"timestamp":"2026-06-04T10:00:01Z","type":"event_msg","payload":{"type":"agent_message","message":"` + message + `","phase":"commentary"}}`,
		`{"timestamp":"2026-06-04T10:00:01Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + message + `"}],"phase":"commentary"}}`,
	}, "\n")))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	if len(parsed.Transcript) != 1 || parsed.Transcript[0].Text == nil || *parsed.Transcript[0].Text != message {
		t.Fatalf("transcript = %#v, want one reconciled message", parsed.Transcript)
	}
}

func TestLoadProviderSessionDetails_LoadsTimestampPrefixedRolloutFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	id := "019e44f4-580e-7f32-981e-1e54ec6907d6"
	writeCodexFixture(t, root, "rollout-2026-05-20T17-35-24-"+id+".jsonl")
	detail, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, id)
	if err != nil {
		t.Fatalf("LoadDetails: %v", err)
	}
	if !strings.HasSuffix(detail.Source.RelativePath, id+".jsonl") {
		t.Fatalf("source = %#v, want timestamp-prefixed rollout", detail.Source)
	}
}

func TestLoadProviderSessionDetails_PrefersExactRolloutWhenBothLayoutsExist(t *testing.T) {
	root := t.TempDir()
	writeCodexFixture(t, root, "rollout-sess_123.jsonl")
	writeCodexFixture(t, root, "rollout-2026-05-20T17-35-24-sess_123.jsonl")
	detail, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, "sess_123")
	if err != nil {
		t.Fatalf("LoadDetails: %v", err)
	}
	if !strings.HasSuffix(detail.Source.RelativePath, "rollout-sess_123.jsonl") {
		t.Fatalf("source = %#v, want exact rollout", detail.Source)
	}
}

func TestLoadProviderSessionDetails_NotFoundIsDistinguishable(t *testing.T) {
	_, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, t.TempDir(), "missing-session")
	if !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestGetProviderSessionDetails_NotFoundIsDistinguishable(t *testing.T) {
	_, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, t.TempDir(), "missing-session")
	if !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestGetProviderSessionDetails_RejectsPathLikeAndMalformedIdentifiers(t *testing.T) {
	for _, id := range []string{"../secret", "/tmp/rollout-session.jsonl", "session.with.dot"} {
		_, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, t.TempDir(), id)
		if !errors.Is(err, providersessions.ErrInvalidIdentifier) {
			t.Fatalf("LoadDetails(%q) error = %v, want ErrInvalidIdentifier", id, err)
		}
	}
}

func TestGetProviderSessionDetails_LoadsCodexSessionFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	writeCodexFixture(t, root, "rollout-sess_123.jsonl")
	detail, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, "sess_123")
	if err != nil || detail.ProviderSession.Provider != providersessions.ProviderCodex || detail.ProviderSession.ID != "sess_123" {
		t.Fatalf("detail = %#v, error = %v", detail, err)
	}
}

func TestGetProviderSessionDetails_LoadsTimestampPrefixedCodexSessionFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	id := "019e44f4-580e-7f32-981e-1e54ec6907d6"
	writeCodexFixture(t, root, "rollout-2026-05-20T17-35-24-"+id+".jsonl")
	detail, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, id)
	if err != nil || !strings.HasSuffix(detail.Source.RelativePath, id+".jsonl") {
		t.Fatalf("detail = %#v, error = %v", detail, err)
	}
}

func TestGetProviderSessionDetails_PrefersExactCodexSessionFileWhenSupportedLayoutsBothExist(t *testing.T) {
	root := t.TempDir()
	writeCodexFixture(t, root, "rollout-sess_123.jsonl")
	writeCodexFixture(t, root, "rollout-2026-05-20T17-35-24-sess_123.jsonl")
	detail, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, "sess_123")
	if err != nil || !strings.HasSuffix(detail.Source.RelativePath, "rollout-sess_123.jsonl") {
		t.Fatalf("detail = %#v, error = %v", detail, err)
	}
}

func TestResolveCodexSessionFile_RejectsAmbiguousTimestampPrefixedMatches(t *testing.T) {
	root := t.TempDir()
	writeCodexFixture(t, root, "rollout-2026-05-20T17-35-24-sess_123.jsonl")
	writeCodexFixtureAt(t, root, "2026/05/19", "rollout-2026-05-20T17-45-24-sess_123.jsonl")
	_, err := Resolve(testFiles, testWalkDirectory, testResolveSymlinks, root, "sess_123")
	if !errors.Is(err, providersessions.ErrAmbiguousSessionFile) {
		t.Fatalf("err = %v, want ErrAmbiguousSessionFile", err)
	}
}

func TestMatchesCodexSessionBaseName_AcceptsSupportedLayoutsOnly(t *testing.T) {
	exact := "rollout-sess_123.jsonl"
	for name, want := range map[string]bool{
		exact: true,
		"rollout-2026-05-20T17-35-24-sess_123.jsonl": true,
		"rollout-backup-sess_123.jsonl":              false,
		"rollout-sess_123.jsonl.bak":                 false,
	} {
		if got := MatchesSessionBaseName(name, "sess_123", exact); got != want {
			t.Fatalf("MatchesSessionBaseName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestGetProviderSessionDetails_IgnoresUnsupportedRolloutFileNames(t *testing.T) {
	root := t.TempDir()
	writeCodexFixture(t, root, "rollout-backup-sess_123.jsonl")
	writeCodexFixture(t, root, "rollout-2026-05-20T17-35-24-backup-sess_123.jsonl")
	_, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, "sess_123")
	if !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestGetProviderSessionDetails_RejectsSessionSymlinkOutsideConfiguredRoot(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	outsidePath := filepath.Join(outside, "rollout-sess-outside.jsonl")
	if err := os.WriteFile(outsidePath, []byte(`{"type":"session_meta"}`), 0o600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}
	linkDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("mkdir link dir: %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(linkDir, "rollout-sess-outside.jsonl")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink capability unavailable: %v", err)
		}
		t.Fatalf("create symlink: %v", err)
	}
	_, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, "sess-outside")
	if !errors.Is(err, providersessions.ErrInvalidIdentifier) {
		t.Fatalf("err = %v, want ErrInvalidIdentifier", err)
	}
}

func TestGetProviderSessionDetails_RejectsSessionSymlinkOutsideConfiguredRootEvenWhenValidMatchExists(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeCodexFixture(t, root, "rollout-sess-shared.jsonl")
	outsidePath := filepath.Join(outside, "rollout-sess-shared.jsonl")
	if err := os.WriteFile(outsidePath, []byte(`{"type":"session_meta"}`), 0o600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}
	linkDir := filepath.Join(root, "2026", "05", "19")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("mkdir link dir: %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(linkDir, "rollout-2026-05-20T17-35-24-sess-shared.jsonl")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink capability unavailable: %v", err)
		}
		t.Fatalf("create symlink: %v", err)
	}
	_, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, "sess-shared")
	if !errors.Is(err, providersessions.ErrInvalidIdentifier) {
		t.Fatalf("err = %v, want ErrInvalidIdentifier", err)
	}
}

func writeCodexFixture(t *testing.T, root, name string) {
	t.Helper()
	writeCodexFixtureAt(t, root, "2026/05/18", name)
}

func writeCodexFixtureAt(t *testing.T, root, relativeDir, name string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(relativeDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"type":"session_meta"}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

func TestParseDetailsTruncatedJSONLReportsBoundedDiagnostic(t *testing.T) {
	parsed, err := ParseDetails(strings.NewReader(`{"type":"turn_context"}` + "\n" + `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial`))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	if parsed.Summary.MalformedLineCount != 1 || len(parsed.Summary.ParseErrors) != 1 {
		t.Fatalf("summary = %#v, want one truncated diagnostic", parsed.Summary)
	}
	if parsed.Summary.ParseErrors[0].Message != diagnosticTruncatedJSONEvent {
		t.Fatalf("parse error = %#v, want truncated JSON diagnostic", parsed.Summary.ParseErrors[0])
	}
	if len(parsed.Transcript) != 0 {
		t.Fatalf("transcript = %#v, want no fabricated entries", parsed.Transcript)
	}
}

func TestParseDetailsKeepsValidLinesWhenLaterLineIsMalformed(t *testing.T) {
	parsed, err := ParseDetails(strings.NewReader(strings.Join([]string{
		`{"type":"event_msg","payload":{"type":"agent_message","message":"kept"}}`,
		`{bad json`,
	}, "\n")))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	if len(parsed.Transcript) != 1 || parsed.Summary.MalformedLineCount != 1 {
		t.Fatalf("parsed = %#v, want one transcript entry and one malformed diagnostic", parsed)
	}
}

func TestParseDetailsSanitizesUnknownEventLabels(t *testing.T) {
	parsed, err := ParseDetails(strings.NewReader(`{"type":"C:\\private\\rollout.jsonl","payload":{"type":"api_key-secret-token"}}` + "\n"))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	if len(parsed.Summary.UnknownEvents) != 1 {
		t.Fatalf("unknown events = %#v, want one sanitized event", parsed.Summary.UnknownEvents)
	}
	event := parsed.Summary.UnknownEvents[0]
	if stringValue(event.Type) != "redacted" || stringValue(event.PayloadType) != "redacted" {
		t.Fatalf("unknown event = %#v, want redacted labels", event)
	}
	encoded := strings.Join([]string{stringValue(event.Type), stringValue(event.PayloadType)}, " ")
	if strings.Contains(encoded, "private") || strings.Contains(encoded, "api_key") {
		t.Fatalf("unknown event leaked sensitive labels: %s", encoded)
	}
}

func TestParseDetailsUnreadableStreamFailsSafely(t *testing.T) {
	_, err := ParseDetails(errorReader{err: errors.New("permission denied reading C:\\secret\\rollout.jsonl")})
	if !errors.Is(err, providersessions.ErrSessionStorageUnavailable) {
		t.Fatalf("err = %v, want ErrSessionStorageUnavailable", err)
	}
}

func TestParseDetailsEnforcesLineAndByteLimitsDeterministically(t *testing.T) {
	t.Run("line limit", func(t *testing.T) {
		restore := overrideCodexInspectionLimits(3, 1<<20, 10, 10)
		t.Cleanup(restore)

		lines := strings.Join([]string{
			`{"type":"event_msg","payload":{"type":"agent_message","message":"one"}}`,
			`{"type":"event_msg","payload":{"type":"agent_message","message":"two"}}`,
			`{"type":"event_msg","payload":{"type":"agent_message","message":"three"}}`,
			`{"type":"event_msg","payload":{"type":"agent_message","message":"four"}}`,
		}, "\n")
		first, err := ParseDetails(strings.NewReader(lines))
		if err != nil {
			t.Fatalf("first ParseDetails: %v", err)
		}
		second, err := ParseDetails(strings.NewReader(lines))
		if err != nil {
			t.Fatalf("second ParseDetails: %v", err)
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("limit handling differed across runs:\nfirst=%#v\nsecond=%#v", first, second)
		}
		if len(first.Transcript) != 3 {
			t.Fatalf("transcript = %#v, want three retained entries before line limit", first.Transcript)
		}
		if first.Summary.ParseErrors[len(first.Summary.ParseErrors)-1].Message != diagnosticInspectionLineLimit {
			t.Fatalf("parse errors = %#v, want line-limit diagnostic", first.Summary.ParseErrors)
		}
	})

	t.Run("byte limit", func(t *testing.T) {
		restore := overrideCodexInspectionLimits(100, 64, 10, 10)
		t.Cleanup(restore)

		oversized := strings.Repeat("x", 80) + "\n"
		limited, err := ParseDetails(strings.NewReader(oversized))
		if err != nil {
			t.Fatalf("byte-limited ParseDetails: %v", err)
		}
		if limited.Summary.ParseErrors[len(limited.Summary.ParseErrors)-1].Message != diagnosticInspectionByteLimit {
			t.Fatalf("parse errors = %#v, want byte-limit diagnostic", limited.Summary.ParseErrors)
		}
	})
}

func TestParseDetailsEnforcesTranscriptAndDiagnosticLimits(t *testing.T) {
	restore := overrideCodexInspectionLimits(100, 1<<20, 1, 1)
	t.Cleanup(restore)

	lines := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		lines = append(lines, `{"type":"event_msg","payload":{"type":"agent_message","message":"msg-`+fmt.Sprint(i)+`"}}`)
	}
	parsed, err := ParseDetails(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	if len(parsed.Transcript) != 1 {
		t.Fatalf("transcript = %#v, want one retained entry", parsed.Transcript)
	}
	if parsed.Summary.ParseErrors[len(parsed.Summary.ParseErrors)-1].Message != diagnosticInspectionTranscriptLimit {
		t.Fatalf("parse errors = %#v, want transcript-limit diagnostic", parsed.Summary.ParseErrors)
	}

	unknownLines := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		unknownLines = append(unknownLines, `{"type":"future_event_`+fmt.Sprint(i)+`"}`)
	}
	unknownParsed, err := ParseDetails(strings.NewReader(strings.Join(unknownLines, "\n")))
	if err != nil {
		t.Fatalf("unknown ParseDetails: %v", err)
	}
	if len(unknownParsed.Summary.UnknownEvents) != 1 {
		t.Fatalf("unknown events = %#v, want one retained diagnostic", unknownParsed.Summary.UnknownEvents)
	}
	if unknownParsed.Summary.UnknownEventCount != 4 {
		t.Fatalf("UnknownEventCount = %d, want counted omissions", unknownParsed.Summary.UnknownEventCount)
	}
}

func TestParseCancellationDuringJSONLLoopReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	reader := &blockAfterLineReader{
		content: []byte(strings.Join([]string{
			`{"type":"event_msg","payload":{"type":"agent_message","message":"first"}}`,
			`{"type":"event_msg","payload":{"type":"agent_message","message":"second"}}`,
			`{"type":"event_msg","payload":{"type":"agent_message","message":"third"}}`,
		}, "\n") + "\n"),
		linesBeforeBlock: 1,
		block:            release,
	}
	done := make(chan error, 1)
	go func() {
		_, err := parseCodexSessionDetails(ctx, reader)
		done <- err
	}()
	cancel()
	close(release)
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}


func TestDiscoveryRejectsExcessiveWalkCandidates(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < maxCodexWalkCandidates+1; i++ {
		writeCodexFixtureAt(t, root, fmt.Sprintf("2026/05/%02d", i), "rollout-session-limit.jsonl")
	}
	_, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, "session-limit")
	if !errors.Is(err, providersessions.ErrSessionStorageUnavailable) {
		t.Fatalf("err = %v, want ErrSessionStorageUnavailable", err)
	}
	if strings.Contains(err.Error(), root) {
		t.Fatalf("public error exposed host path: %v", err)
	}
}

func TestLoadDetailsUnreadableFileFailsWithoutHostPathLeakage(t *testing.T) {
	root := t.TempDir()
	hostPath := filepath.Join(root, "2026", "05", "18", "rollout-session-read.jsonl")
	if err := os.MkdirAll(filepath.Dir(hostPath), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(hostPath, []byte(`{"type":"session_meta"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	files := &errorOpenFileSystem{
		base:      platformfilesystem.Local{},
		openError: errors.New("permission denied reading " + hostPath),
	}
	_, err := LoadDetails(files, testWalkDirectory, testResolveSymlinks, root, "session-read")
	if !errors.Is(err, providersessions.ErrSessionStorageUnavailable) {
		t.Fatalf("err = %v, want ErrSessionStorageUnavailable", err)
	}
	if strings.Contains(err.Error(), hostPath) || strings.Contains(err.Error(), root) {
		t.Fatalf("public error exposed host path: %v", err)
	}
}

type blockAfterLineReader struct {
	content          []byte
	offset           int
	linesDelivered   int
	linesBeforeBlock int
	block            <-chan struct{}
}

func (r *blockAfterLineReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.content) {
		return 0, io.EOF
	}
	end := r.offset
	for end < len(r.content) && r.content[end] != '\n' {
		end++
	}
	if end < len(r.content) {
		end++
	}
	if r.linesDelivered >= r.linesBeforeBlock {
		<-r.block
	}
	chunk := r.content[r.offset:end]
	r.offset = end
	r.linesDelivered++
	n := copy(p, chunk)
	return n, nil
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

type errorOpenFileSystem struct {
	base      providersessions.FileSystem
	openError error
}

func (f *errorOpenFileSystem) Open(path string) (io.ReadCloser, error) {
	if f.openError != nil {
		return nil, f.openError
	}
	return f.base.Open(path)
}

func (f *errorOpenFileSystem) Stat(path string) (fs.FileInfo, error) {
	return f.base.Stat(path)
}

func overrideCodexInspectionLimits(lines int, bytes int64, transcript int, diagnostics int) func() {
	previousLines := maxCodexJSONLLinesPerInspection
	previousBytes := maxCodexJSONLBytesPerInspection
	previousTranscript := maxCodexTranscriptEntries
	previousDiagnostics := maxCodexDiagnosticRecords
	maxCodexJSONLLinesPerInspection = lines
	maxCodexJSONLBytesPerInspection = bytes
	maxCodexTranscriptEntries = transcript
	maxCodexDiagnosticRecords = diagnostics
	return func() {
		maxCodexJSONLLinesPerInspection = previousLines
		maxCodexJSONLBytesPerInspection = previousBytes
		maxCodexTranscriptEntries = previousTranscript
		maxCodexDiagnosticRecords = previousDiagnostics
	}
}

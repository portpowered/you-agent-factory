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
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
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
		first, firstErr := ParseDetails(strings.NewReader(lines))
		if firstErr != nil {
			t.Fatalf("first ParseDetails error = %v, want resource-limit cause", firstErr)
		}
		second, secondErr := ParseDetails(strings.NewReader(lines))
		if secondErr != nil {
			t.Fatalf("second ParseDetails error = %v, want resource-limit cause", secondErr)
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
			t.Fatalf("byte-limited ParseDetails error = %v, want resource-limit cause", err)
		}
		if limited.Summary.ParseErrors[len(limited.Summary.ParseErrors)-1].Message != diagnosticInspectionByteLimit {
			t.Fatalf("parse errors = %#v, want byte-limit diagnostic", limited.Summary.ParseErrors)
		}
	})
}

func TestParseDetailsHonorsInclusiveByteLimitBoundary(t *testing.T) {
	content := `{"type":"event_msg","payload":{"type":"agent_message","message":"complete"}}` + "\n"
	limit := int64(len(content))
	sessionID := "exact-cap-session"

	t.Run("exact cap succeeds", func(t *testing.T) {
		restore := overrideCodexInspectionLimits(100, limit, 10, 10)
		t.Cleanup(restore)

		parsed, err := parseCodexSessionDetailsForSession(context.Background(), strings.NewReader(content), sessionID)
		if err != nil {
			t.Fatalf("parseCodexSessionDetailsForSession error = %v, want exact-cap success", err)
		}
		if parsed.Summary.EventCount != 1 || len(parsed.Transcript) != 1 {
			t.Fatalf("parsed = %#v, want the exact-cap completion record retained", parsed)
		}
	})

	t.Run("cap plus one fails with bounded context", func(t *testing.T) {
		restore := overrideCodexInspectionLimits(100, limit, 10, 10)
		t.Cleanup(restore)

		parsed, err := parseCodexSessionDetailsForSession(context.Background(), strings.NewReader(content+"x"), sessionID)
		if err != nil {
			t.Fatalf("parseCodexSessionDetailsForSession error = %v, want resource-limit cause", err)
		}
		if len(parsed.Summary.ParseErrors) != 1 || parsed.Summary.ParseErrors[0].Message != diagnosticInspectionByteLimit {
			t.Fatalf("missing bounded byte diagnostic: %#v", parsed.Summary)
		}
		if parsed.Summary.EventCount != 1 {
			t.Fatalf("parsed summary = %#v, want the valid record before cap+1 retained", parsed.Summary)
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
		t.Fatalf("unknown ParseDetails error = %v, want diagnostic-limit cause", err)
	}
	if len(unknownParsed.Summary.UnknownEvents) != 0 || len(unknownParsed.Summary.ParseErrors) != 1 || unknownParsed.Summary.ParseErrors[0].Message != diagnosticInspectionDiagnosticLimit {
		t.Fatalf("unknown events = %#v, want one retained diagnostic", unknownParsed.Summary.UnknownEvents)
	}
	if unknownParsed.Summary.UnknownEventCount < 1 {
		t.Fatalf("UnknownEventCount = %d, want at least one counted event", unknownParsed.Summary.UnknownEventCount)
	}
}

func TestParseDetailsContinuesAfterTranscriptLimit(t *testing.T) {
	restore := overrideCodexInspectionLimits(100, 1<<20, 1, 10)
	t.Cleanup(restore)

	parsed, err := ParseDetails(strings.NewReader(strings.Join([]string{
		`{"type":"event_msg","payload":{"type":"agent_message","message":"first"}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"second"}}`,
		`{"type":"response_item","payload":{"type":"function_call","call_id":"call-after-transcript-limit","name":"exec_command","arguments":"go test"}}`,
	}, "\n")))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	if len(parsed.Transcript) != 1 {
		t.Fatalf("transcript = %#v, want one retained entry", parsed.Transcript)
	}
	if len(parsed.Summary.FunctionCalls) != 1 || parsed.Summary.EventCount != 3 {
		t.Fatalf("summary = %#v, want later function call and all events retained", parsed.Summary)
	}
	if parsed.Summary.ParseErrors[len(parsed.Summary.ParseErrors)-1].Message != diagnosticInspectionTranscriptLimit {
		t.Fatalf("parse errors = %#v, want transcript-limit diagnostic", parsed.Summary.ParseErrors)
	}
}

func TestParseDetailsRejectsOversizedPhysicalLineWithSafeCause(t *testing.T) {
	restore := overrideCodexJSONLLineLimit(32)
	t.Cleanup(restore)

	content := strings.Repeat("rollout-secret-", 8) + "\n"
	parsed, err := ParseDetails(strings.NewReader(content))
	if err != nil {
		t.Fatalf("ParseDetails error = %v, want record-limit cause", err)
	}
	if len(parsed.Summary.ParseErrors) != 1 || parsed.Summary.ParseErrors[0].Message != diagnosticInspectionRecordLimit {
		t.Fatalf("parse errors = %#v, want bounded diagnostics", parsed.Summary.ParseErrors)
	}
}

func TestParseDetailsBoundsRetainedText(t *testing.T) {
	restore := overrideCodexRetainedTextLimit(32)
	t.Cleanup(restore)

	parsed, err := ParseDetails(strings.NewReader(
		`{"type":"event_msg","payload":{"type":"agent_message","message":"` + strings.Repeat("x", 128) + `"}}`,
	))
	if err != nil {
		t.Fatalf("ParseDetails: %v", err)
	}
	if len(parsed.Transcript) != 1 || parsed.Transcript[0].Text == nil {
		t.Fatalf("transcript = %#v, want one bounded text entry", parsed.Transcript)
	}
	if len(*parsed.Transcript[0].Text) > 32 {
		t.Fatalf("transcript text length = %d, want <= 32", len(*parsed.Transcript[0].Text))
	}
	if parsed.Summary.ParseErrors[len(parsed.Summary.ParseErrors)-1].Message != diagnosticInspectionRetainedTextLimit {
		t.Fatalf("parse errors = %#v, want retained-output diagnostic", parsed.Summary.ParseErrors)
	}
}

func TestLoadDetailsLimitErrorIncludesSafeSessionContext(t *testing.T) {
	restore := overrideCodexInspectionLimits(100, 64, 10, 10)
	t.Cleanup(restore)

	root := t.TempDir()
	sessionID := "large-rollout-session"
	dir := filepath.Join(root, "2026", "07", "27")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "rollout-"+sessionID+".jsonl"),
		[]byte(strings.Repeat("rollout-secret-", 8)),
		0o600,
	); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	detail, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, sessionID)
	if err != nil {
		t.Fatalf("LoadDetails error = %v, want resource-limit cause", err)
	}
	if detail.ProviderSession.ID != sessionID || len(detail.Parse.ParseErrors) != 1 || detail.Parse.ParseErrors[0].Message != diagnosticInspectionByteLimit {
		t.Fatalf("detail missing identity or bounded diagnostic: %#v", detail)
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
	base      providersessionsinternal.FileSystem
	openError error
	statError error
}

func (f *errorOpenFileSystem) Open(path string) (io.ReadCloser, error) {
	if f.openError != nil {
		return nil, f.openError
	}
	return f.base.Open(path)
}

func (f *errorOpenFileSystem) Stat(path string) (fs.FileInfo, error) {
	if f.statError != nil {
		return nil, f.statError
	}
	return f.base.Stat(path)
}

func TestReaderDetailsRetainsValidEntryBeforeTruncatedEOF(t *testing.T) {
	root, id := writeCodexJSONLFixture(t, `{"type":"event_msg","payload":{"type":"agent_message","message":"kept"}}`+"\n"+`{"type":"response_item","payload":`)
	reader := newTestReader(t, testFiles, testWalkDirectory, testResolveSymlinks, root)
	detail, err := reader.Details(context.Background(), codexSessionRef(id))
	if err != nil {
		t.Fatalf("Details: %v", err)
	}
	if detail.ProviderSession.ID != id || len(detail.Transcript) != 1 || stringValue(detail.Transcript[0].Text) != "kept" {
		t.Fatalf("detail = %#v, want exact identity and retained valid entry", detail)
	}
	if detail.Parse.MalformedLineCount != 1 || len(detail.Parse.ParseErrors) != 1 || detail.Parse.ParseErrors[0].Message != diagnosticTruncatedJSONEvent {
		t.Fatalf("parse = %#v, want one truncated diagnostic", detail.Parse)
	}
}

type cancelingReadFileSystem struct {
	providersessionsinternal.FileSystem
	cancel context.CancelFunc
	closed bool
}

func (f *cancelingReadFileSystem) Open(path string) (io.ReadCloser, error) {
	file, err := f.FileSystem.Open(path)
	if err != nil {
		return nil, err
	}
	return &cancelingReadStream{ReadCloser: file, owner: f}, nil
}

type cancelingReadStream struct {
	io.ReadCloser
	owner *cancelingReadFileSystem
}

func (s *cancelingReadStream) Read(p []byte) (int, error) {
	n, err := s.ReadCloser.Read(p)
	s.owner.cancel()
	return n, err
}

func (s *cancelingReadStream) Close() error {
	s.owner.closed = true
	return s.ReadCloser.Close()
}

func TestReaderDetailsCancellationDuringReadClosesStream(t *testing.T) {
	root, id := writeCodexJSONLFixture(t, representativeCodexJSONL())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	files := &cancelingReadFileSystem{FileSystem: testFiles, cancel: cancel}
	reader := newTestReader(t, files, testWalkDirectory, testResolveSymlinks, root)
	_, err := reader.Details(ctx, codexSessionRef(id))
	if !errors.Is(err, context.Canceled) || !files.closed {
		t.Fatalf("Details = %v, stream closed=%v; want cancellation and closed stream", err, files.closed)
	}
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

func overrideCodexJSONLLineLimit(limit int64) func() {
	previous := maxCodexJSONLLineBytes
	maxCodexJSONLLineBytes = limit
	return func() { maxCodexJSONLLineBytes = previous }
}

func overrideCodexRetainedTextLimit(limit int64) func() {
	previous := maxCodexRetainedTextBytes
	maxCodexRetainedTextBytes = limit
	return func() { maxCodexRetainedTextBytes = previous }
}

func TestParseDetailsRecoversOversizedRecordAndPreservesOrder(t *testing.T) {
	message := func(text string) string {
		return `{"type":"event_msg","payload":{"type":"agent_message","message":"` + text + `"}}` + "\n"
	}
	large := strings.Repeat("v", 128<<10)
	parsed, err := ParseDetails(strings.NewReader(message(large) + strings.Repeat("secret", 200000) + "\n" + message("after")))
	if err != nil || len(parsed.Transcript) != 2 || stringValue(parsed.Transcript[0].Text) != large || stringValue(parsed.Transcript[1].Text) != "after" {
		t.Fatalf("lost ordered valid neighbors: %v, %#v", err, parsed.Transcript)
	}
	if len(parsed.Summary.ParseErrors) != 1 || parsed.Summary.ParseErrors[0].LineNumber != 2 || parsed.Summary.ParseErrors[0].Message != diagnosticInspectionRecordLimit {
		t.Fatalf("missing safe physical-line diagnostic: %#v", parsed.Summary)
	}
}

func TestParseDetailsDiagnosticOverflowIsBoundedPartialSuccess(t *testing.T) {
	restore := overrideCodexInspectionLimits(100, 1<<20, 10, 3)
	t.Cleanup(restore)
	for _, content := range []string{strings.Repeat("{bad\n", 10), strings.Repeat(`{"type":"future"}`+"\n", 10)} {
		parsed, err := ParseDetails(strings.NewReader(content))
		if err != nil || len(parsed.Summary.ParseErrors)+len(parsed.Summary.UnknownEvents) != 3 || len(parsed.Transcript) != 0 {
			t.Fatalf("diagnostic-only response not bounded: %v, %#v", err, parsed)
		}
		last := parsed.Summary.ParseErrors[len(parsed.Summary.ParseErrors)-1]
		if last.LineNumber != 4 || last.Message != diagnosticInspectionDiagnosticLimit {
			t.Fatalf("missing terminal outcome: %#v", last)
		}
	}
}

func TestReaderDetailsSnapshotsGrowingFileAndRereadsCompletedTail(t *testing.T) {
	prefix := `{"type":"event_msg","payload":{"type":"agent_message","message":"before"}}` + "\n"
	tail := `{"type":"event_msg","payload":{"type":"agent_message","message":"after`
	root, id := writeCodexJSONLFixture(t, prefix+tail)
	files := &appendOnOpenFileSystem{FileSystem: testFiles, tail: `"}}` + "\n"}
	first, err := LoadDetails(files, testWalkDirectory, testResolveSymlinks, root, id)
	if err != nil || len(first.Transcript) != 1 || len(first.Parse.ParseErrors) != 1 || first.Parse.ParseErrors[0].Message != diagnosticTruncatedJSONEvent {
		t.Fatalf("snapshot followed append: %v, %#v", err, first)
	}
	second, err := LoadDetails(files, testWalkDirectory, testResolveSymlinks, root, id)
	if err != nil || len(second.Transcript) != 2 || stringValue(second.Transcript[1].Text) != "after" || len(second.Parse.ParseErrors) != 0 {
		t.Fatalf("reread lost completed tail: %v, %#v", err, second)
	}
	if len(first.Transcript) != 1 || first.Source.SizeBytes != int64(len(prefix+tail)) {
		t.Fatalf("first snapshot mutated: %#v", first)
	}
}

type appendOnOpenFileSystem struct {
	providersessionsinternal.FileSystem
	tail string
}

func (f *appendOnOpenFileSystem) Open(path string) (io.ReadCloser, error) {
	file, err := f.FileSystem.Open(path)
	if err != nil {
		return nil, err
	}
	return &appendBeforeReadStream{ReadCloser: file, owner: f, path: path}, nil
}

type appendBeforeReadStream struct {
	io.ReadCloser
	owner *appendOnOpenFileSystem
	path  string
}

func (s *appendBeforeReadStream) Read(p []byte) (int, error) {
	if s.owner.tail != "" {
		writer, err := os.OpenFile(s.path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return 0, err
		}
		_, err = writer.WriteString(s.owner.tail)
		closeErr := writer.Close()
		if err != nil {
			return 0, err
		}
		if closeErr != nil {
			return 0, closeErr
		}
		s.owner.tail = ""
	}
	return s.ReadCloser.Read(p)
}

func TestParseDetailsSafeUnderlyingStorageCause(t *testing.T) {
	cause := fmt.Errorf("reading C:\\secret\\rollout: %w", fs.ErrPermission)
	_, err := ParseDetails(errorReader{err: cause})
	if !errors.Is(err, providersessions.ErrSessionStorageUnavailable) || !errors.Is(err, fs.ErrPermission) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe or missing cause: %v", err)
	}
}

func TestParseDetailsReadsAtMostByteBudgetAndOneProbe(t *testing.T) {
	restore := overrideCodexInspectionLimits(100, 100, 10, 10)
	t.Cleanup(restore)
	reader := &countingCodexReader{Reader: strings.NewReader(strings.Repeat("x", 1000))}
	parsed, err := ParseDetails(reader)
	if err != nil || reader.read != 101 || len(parsed.Summary.ParseErrors) != 1 || parsed.Summary.ParseErrors[0].Message != diagnosticInspectionByteLimit {
		t.Fatalf("unbounded inspection: read=%d err=%v summary=%#v", reader.read, err, parsed.Summary)
	}
}

type countingCodexReader struct {
	io.Reader
	read int
}

func (r *countingCodexReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read += n
	return n, err
}

func TestParseDetailsRecordBoundariesAtNewlineAndEOF(t *testing.T) {
	restore := overrideCodexJSONLLineLimit(128)
	t.Cleanup(restore)
	base := `{"type":"session_meta"}`
	for _, ending := range []string{"", "\n"} {
		exact := base + strings.Repeat(" ", 128-len(base)-len(ending)) + ending
		parsed, err := ParseDetails(strings.NewReader(exact))
		if err != nil || parsed.Summary.EventCount != 1 || len(parsed.Summary.ParseErrors) != 0 {
			t.Fatalf("exact record boundary: %v %#v", err, parsed)
		}
		parsed, err = ParseDetails(strings.NewReader(base + strings.Repeat(" ", 129-len(base)-len(ending)) + ending))
		if err != nil || len(parsed.Summary.ParseErrors) != 1 || parsed.Summary.ParseErrors[0].LineNumber != 1 || parsed.Summary.ParseErrors[0].Message != diagnosticInspectionRecordLimit {
			t.Fatalf("oversized record boundary: %v %#v", err, parsed)
		}
	}
}

func TestParseDetailsExactLineBudgetPreservesFinalJSONWithoutNewline(t *testing.T) {
	restore := overrideCodexInspectionLimits(2, 1<<20, 10, 10)
	t.Cleanup(restore)
	parsed, err := ParseDetails(strings.NewReader(`{"type":"session_meta"}` + "\n" + `{"type":"event_msg","payload":{"type":"agent_message","message":"final"}}`))
	if err != nil || len(parsed.Summary.ParseErrors) != 0 || len(parsed.Transcript) != 1 || stringValue(parsed.Transcript[0].Text) != "final" {
		t.Fatalf("exact line boundary: %v %#v", err, parsed)
	}
}

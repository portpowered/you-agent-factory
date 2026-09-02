package stdio_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	runtimeartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	"github.com/portpowered/infinite-you/pkg/platform/wiretranscript"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestServeACPWritesAWireTranscriptByDefault proves the customer-facing
// promise of the ACP wire log: after running `you server acp`, the traffic in
// both directions is on disk without anyone having enabled anything, and the
// transcript reproduces exactly what crossed the wire.
//
// The transcript is checked against the process' own stdout rather than
// against a hand-written expectation, so this cannot pass by agreeing with a
// stale fixture.
//
// The public process path proves the live publication boundary after each
// response, including a configured rolling rotation. The test supplies the
// exact ACP wire-recorder external effect through edges.Edges; the recorder
// itself remains the production wiretranscript.Opener and rolling writer.
func TestServeACPWritesAWireTranscriptByDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving you server acp through root.BuildProcess")
	}
	t.Parallel()
	home := t.TempDir()
	cwd := t.TempDir()
	environment := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	runner := newTranscriptProviderRunner()
	process := newTranscriptProcess(t, home, cwd, runner)

	stdinRead, stdinWrite := io.Pipe()
	stdoutRead, stdoutWrite := io.Pipe()

	var stderr bytes.Buffer
	command := support.StartProcessCommand(t, process, root.Input{
		Args:             []string{"you", "server", "acp"},
		Env:              environment,
		Stdin:            stdinRead,
		Stdout:           stdoutWrite,
		Stderr:           &stderr,
		WorkingDirectory: cwd,
	})
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
	})

	stdout := &transcriptRPCReader{reader: bufio.NewReader(stdoutRead)}
	sent := exerciseTranscriptConversation(t, home, cwd, stdinWrite, stdout)
	finishTranscriptConversation(t, home, command, stdinWrite, stdoutWrite, runner, stdout, sent)
}

func newTranscriptProviderRunner() *support.ShapedProviderCommandRunner {
	providerResult := platformprocess.CommandResult{Stdout: []byte(fixtureFinalAnswerText)}
	return support.NewShapedProviderCommandRunner(providerResult, providerResult, providerResult, providerResult)
}

func newTranscriptProcess(
	t *testing.T,
	home string,
	cwd string,
	runner *support.ShapedProviderCommandRunner,
) support.Process {
	t.Helper()
	seedFixtureFactory(t, cwd)
	support.SeedACPAgentProfile(t, home, fixtureFactoryTargetID, []string{fixtureFactoryTargetID})
	return support.BuildProcess(t, serviceedges.Edges{
		ACPWireRecorder:                    newRotatingWireRecorder(t, home),
		ProviderCommandRunner:              runner,
		FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
	})
}

func exerciseTranscriptConversation(
	t *testing.T,
	home string,
	cwd string,
	stdinWrite *io.PipeWriter,
	stdout *transcriptRPCReader,
) []string {
	t.Helper()
	var sent []string
	send := func(line string) {
		sent = append(sent, line)
		writeRPCLine(t, stdinWrite, line)
	}
	send(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":%s}`, fixtureInitializeParams))
	if response := stdout.readResponse(t); response.Error != nil {
		t.Fatalf("initialize response error = %+v, want a response", response.Error)
	}
	assertOutboundTranscriptVisible(t, home, stdout.frames)

	// Large rejected frames force the configured one-megabyte rolling boundary
	// through the real TeeReader and rolling transcript while remaining a
	// customer-visible malformed-frame exchange.
	const largeMalformedFrameCount = 5
	largeMalformedLine := strings.Repeat("x", 600*1024)
	for index := 0; index < largeMalformedFrameCount; index++ {
		send(largeMalformedLine)
		if response := stdout.readResponse(t); response.Error == nil {
			t.Fatalf("malformed request %d response error = nil, want rejection", index+1)
		}
		assertOutboundTranscriptVisible(t, home, stdout.frames)
	}

	send(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":%q,"mcpServers":[]}}`, cwd))
	newSessionResponse := stdout.readResponse(t)
	if newSessionResponse.Error != nil {
		t.Fatalf("session/new response error = %+v, want a successful result", newSessionResponse.Error)
	}
	var session acpsdk.NewSessionResponse
	if err := json.Unmarshal(newSessionResponse.Result, &session); err != nil {
		t.Fatalf("unmarshal session/new result: %v", err)
	}
	if session.SessionId == "" {
		t.Fatal("session/new returned a blank sessionId")
	}
	assertOutboundTranscriptVisible(t, home, stdout.frames)

	for turn := 0; turn < 2; turn++ {
		promptParams, err := json.Marshal(map[string]any{
			"sessionId": session.SessionId,
			"prompt":    []map[string]any{{"type": "text", "text": fmt.Sprintf("rotation turn %d", turn+1)}},
		})
		if err != nil {
			t.Fatalf("marshal prompt params: %v", err)
		}
		send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"session/prompt","params":%s}`, turn+3, promptParams))
		promptResponse, notifications := stdout.readNotificationsUntilResponse(t)
		if got := agentMessageChunkTextFrom(t, notifications); got != fixtureFinalAnswerText {
			t.Fatalf("turn %d assistant text = %q, want %q", turn+1, got, fixtureFinalAnswerText)
		}
		if promptResponse.Error != nil {
			t.Fatalf("turn %d prompt response error = %+v, want success", turn+1, promptResponse.Error)
		}
		var promptResult acpsdk.PromptResponse
		if err := json.Unmarshal(promptResponse.Result, &promptResult); err != nil {
			t.Fatalf("unmarshal turn %d prompt result: %v", turn+1, err)
		}
		if promptResult.StopReason != acpsdk.StopReasonEndTurn {
			t.Fatalf("turn %d stopReason = %q, want %q", turn+1, promptResult.StopReason, acpsdk.StopReasonEndTurn)
		}
		assertOutboundTranscriptVisible(t, home, stdout.frames)
	}
	return sent
}

func finishTranscriptConversation(
	t *testing.T,
	home string,
	command *support.ProcessCommand,
	stdinWrite *io.PipeWriter,
	stdoutWrite *io.PipeWriter,
	runner *support.ShapedProviderCommandRunner,
	stdout *transcriptRPCReader,
	sent []string,
) {
	t.Helper()
	if err := stdinWrite.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	<-command.Done()
	if err := command.Err(); err != nil {
		t.Fatalf("Process.Execute(you server acp) error = %v", err)
	}
	if err := stdoutWrite.Close(); err != nil {
		t.Fatalf("close stdout: %v", err)
	}
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("provider command call count = %d, want one call per prompt turn", got)
	}

	records := readWireTranscript(t, home)
	if len(records) == 0 {
		t.Fatal("no wire transcript records were written")
	}
	if paths := wireTranscriptFiles(t, wiretranscript.Root(home)); len(paths) < 2 {
		t.Fatalf("wire transcript files = %v, want an active file and at least one rotated backup", paths)
	}

	assertTranscriptMatchesTraffic(t, records, sent, stdout.received)
}

func newRotatingWireRecorder(t *testing.T, home string) acp.WireRecorder {
	t.Helper()
	paths, err := runtimeartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("runtimeartifact.NewReserver(): %v", err)
	}
	opener, err := wiretranscript.NewOpener(paths, functionalWireClock{})
	if err != nil {
		t.Fatalf("wiretranscript.NewOpener(): %v", err)
	}
	return func(connectionID string) (acp.WireTranscript, error) {
		return opener.Open(wiretranscript.OpeningRequest{
			RootDirectory: wiretranscript.Root(home),
			ConnectionID:  connectionID,
			StartTimeUTC:  functionalWireClock{}.Now(),
			Config: wiretranscript.Config{
				MaxSizeMB:  1,
				MaxBackups: 4,
				MaxAgeDays: 7,
				Compress:   false,
			},
		})
	}
}

type functionalWireClock struct{}

func (functionalWireClock) Now() time.Time {
	return time.Date(2026, time.August, 15, 12, 13, 14, 0, time.UTC)
}

type transcriptRPCReader struct {
	reader   *bufio.Reader
	received []string
	frames   int
}

func (r *transcriptRPCReader) readFrame(t *testing.T) rpcFrame {
	t.Helper()
	raw, err := r.reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read RPC line: %v", err)
	}
	line := strings.TrimSpace(raw)
	r.received = append(r.received, line)
	r.frames++
	assertLineIsProtocolFrame(t, line)
	var frame rpcFrame
	if err := json.Unmarshal([]byte(line), &frame); err != nil {
		t.Fatalf("unmarshal RPC line %q: %v", line, err)
	}
	return frame
}

func (r *transcriptRPCReader) readResponse(t *testing.T) rpcFrame {
	t.Helper()
	for {
		frame := r.readFrame(t)
		if frame.Method == "" {
			return frame
		}
		if frame.Method != string(acpsdk.ClientMethodSessionUpdate) {
			t.Fatalf("notification method = %q, want %q", frame.Method, acpsdk.ClientMethodSessionUpdate)
		}
	}
}

func (r *transcriptRPCReader) readNotificationsUntilResponse(t *testing.T) (rpcFrame, []acpsdk.SessionNotification) {
	t.Helper()
	var notifications []acpsdk.SessionNotification
	for {
		frame := r.readFrame(t)
		if frame.Method == "" {
			return frame, notifications
		}
		if frame.Method != string(acpsdk.ClientMethodSessionUpdate) {
			t.Fatalf("notification method = %q, want %q", frame.Method, acpsdk.ClientMethodSessionUpdate)
		}
		var notification acpsdk.SessionNotification
		if err := json.Unmarshal(frame.Params, &notification); err != nil {
			t.Fatalf("unmarshal session/update params: %v", err)
		}
		notifications = append(notifications, notification)
	}
}

func assertOutboundTranscriptVisible(t *testing.T, home string, want int) {
	t.Helper()
	records := readWireTranscript(t, home)
	got := 0
	for _, record := range records {
		if record.Direction == wiretranscript.DirectionOut {
			got++
		}
	}
	if got < want {
		t.Fatalf("outbound transcript records visible after stdout response = %d, want at least %d", got, want)
	}
}

// readWireTranscript locates and decodes the active transcript and any rolling
// backups this connection produced under home, then restores wire order from
// each record's sequence number.
func readWireTranscript(t *testing.T, home string) []wiretranscript.Record {
	t.Helper()

	root := wiretranscript.Root(home)
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(paths) == 0 {
		t.Fatalf("no transcript files under %s", root)
	}

	var records []wiretranscript.Record
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("open transcript %s: %v", path, err)
		}
		fileRecords, err := wiretranscript.ReadAll(file)
		_ = file.Close()
		if err != nil {
			t.Fatalf("decode transcript %s: %v", path, err)
		}
		records = append(records, fileRecords...)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Sequence < records[j].Sequence })
	return records
}

// assertTranscriptMatchesTraffic checks the recording against the real traffic
// in both directions.
func assertTranscriptMatchesTraffic(t *testing.T, records []wiretranscript.Record, sent, received []string) {
	t.Helper()

	for index, record := range records {
		if record.Sequence != uint64(index+1) {
			t.Fatalf("record[%d].Sequence = %d, want %d", index, record.Sequence, index+1)
		}
		if record.Version != wiretranscript.FormatVersion {
			t.Fatalf("record[%d].Version = %d, want %d", index, record.Version, wiretranscript.FormatVersion)
		}
	}

	var inbound, outbound []wiretranscript.Record
	var malformed int
	for _, record := range records {
		text := string(record.Frame)
		if record.Frame == nil {
			text = record.Text
		}
		if record.Direction == wiretranscript.DirectionIn && !json.Valid([]byte(text)) {
			malformed++
		}
		switch record.Direction {
		case wiretranscript.DirectionIn:
			if record.Peer != wiretranscript.PeerClient {
				t.Fatalf("inbound record attributed to %q, want client", record.Peer)
			}
			inbound = append(inbound, record)
		case wiretranscript.DirectionOut:
			if record.Peer != wiretranscript.PeerAgent {
				t.Fatalf("outbound record attributed to %q, want agent", record.Peer)
			}
			outbound = append(outbound, record)
		}
	}

	assertRecordedLinesMatch(t, "inbound", inbound, sent)
	assertRecordedLinesMatch(t, "outbound", outbound, received)

	wantMalformed := 0
	for _, line := range sent {
		if !json.Valid([]byte(strings.TrimSpace(line))) {
			wantMalformed++
		}
	}
	if malformed != wantMalformed {
		t.Fatalf("records flagged malformed = %d, want %d", malformed, wantMalformed)
	}
}

// assertRecordedLinesMatch compares every recorded wire frame with the actual
// frame observed by the process. Oversized frames are intentionally retained
// with a bounded text prefix, so those records compare their original byte
// count and prefix while ordinary frames compare normalized JSON exactly.
func assertRecordedLinesMatch(t *testing.T, direction string, recorded []wiretranscript.Record, actual []string) {
	t.Helper()
	if len(recorded) != len(actual) {
		t.Fatalf("%s records = %d, want %d\n recorded: %v\n actual: %v",
			direction, len(recorded), len(actual), recorded, actual)
	}
	for index, record := range recorded {
		line := actual[index]
		if record.Bytes != len(line)+1 {
			t.Fatalf("%s record[%d] byte count = %d, want %d", direction, index, record.Bytes, len(line)+1)
		}
		if record.Frame == nil && record.Bytes > len(record.Text)+1 {
			if !strings.HasPrefix(line, record.Text) {
				t.Fatalf("%s record[%d] prefix differs\n recorded %s\n actual prefix %s",
					direction, index, record.Text, line[:min(len(line), len(record.Text))])
			}
			continue
		}
		recordedLine := string(record.Frame)
		if record.Frame == nil {
			recordedLine = record.Text
		}
		if normalizeJSONLine(recordedLine) != normalizeJSONLine(line) {
			t.Fatalf("%s record[%d] differs\n recorded %s\n actual   %s",
				direction, index, recordedLine, line)
		}
	}
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func normalizeJSONLine(line string) string {
	trimmed := strings.TrimSpace(line)
	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
		return trimmed
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return trimmed
	}
	return string(normalized)
}

func TestServeACPDoesNotRecordFailedOutboundFrame(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving you server acp through root.BuildProcess")
	}
	t.Parallel()
	home := t.TempDir()
	cwd := t.TempDir()
	environment := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)

	process := support.BuildProcess(t, serviceedges.Edges{
		ACPWireRecorder:                    newRotatingWireRecorder(t, home),
		FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
	})

	stdoutErr := errors.New("stdout failed")
	var stderr bytes.Buffer
	err := process.Execute(root.Input{
		Args:             []string{"you", "server", "acp"},
		Env:              environment,
		Stdin:            strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":%s}`, fixtureInitializeParams) + "\n"),
		Stdout:           failingWriter{err: stdoutErr},
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: cwd,
	})
	if err == nil || !strings.Contains(err.Error(), "connection ended with an error") {
		t.Fatalf("you server acp Execute() error = %v, want the public connection-error diagnostic", err)
	}

	records := readWireTranscript(t, home)
	if len(records) != 1 || records[0].Direction != wiretranscript.DirectionIn {
		t.Fatalf("records after failed stdout = %+v, want only the inbound initialize frame", records)
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

// TestServeACPWireTranscriptIsOwnerReadableOnly proves the permission half of
// the wire-log contract in docs/reference/serve-acp.md: "One file per
// connection, mode 0600".
//
// The mode is the only thing standing between the transcript and any other
// user on the host, and the same guide is explicit about what the file holds:
// "The log contains full prompt and response content. It is a transcript of
// the session, not a sanitized diagnostic." Recording is also on by default,
// so a customer who never opted in still gets one of these per connection.
func TestServeACPWireTranscriptIsOwnerReadableOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving you server acp through root.BuildProcess")
	}
	t.Parallel()
	home := t.TempDir()
	cwd := t.TempDir()
	environment := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)

	process := support.BuildProcess(t, serviceedges.Edges{
		FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
	})

	var stdout, stderr bytes.Buffer
	if err := process.Execute(root.Input{
		Args:             []string{"you", "server", "acp"},
		Env:              environment,
		Stdin:            strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":%s}`, fixtureInitializeParams) + "\n"),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: cwd,
	}); err != nil {
		t.Fatalf("you server acp Execute() error = %v", err)
	}

	transcripts := wireTranscriptFiles(t, filepath.Join(home, ".you-agent-factory", "acp-wire"))
	if len(transcripts) != 1 {
		t.Fatalf("wire transcripts = %v, want exactly one for one connection", transcripts)
	}
	info, err := os.Stat(transcripts[0])
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", transcripts[0], err)
	}
	if runtime.GOOS == "windows" {
		// Windows exposes profile ACL protection rather than POSIX owner-only
		// permission bits; os.Chmod(0600) therefore reads back as 0666.
		if perm := info.Mode().Perm(); perm&0o600 != 0o600 {
			t.Fatalf("wire transcript mode = %#o, want owner read/write retained", perm)
		}
		return
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("wire transcript mode = %#o, want %#o: the transcript holds full prompt and response content",
			perm, 0o600)
	}
}

// wireTranscriptFiles returns every transcript written under root.
func wireTranscriptFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk %q: %v", root, err)
	}
	return files
}

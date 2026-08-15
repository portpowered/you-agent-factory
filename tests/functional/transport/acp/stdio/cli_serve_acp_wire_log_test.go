package stdio_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/wiretranscript"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestServeACPWritesAWireTranscriptByDefault proves the customer-facing
// promise of the ACP wire log: after running `you serve acp`, the traffic in
// both directions is on disk without anyone having enabled anything, and the
// transcript reproduces exactly what crossed the wire.
//
// The transcript is checked against the process' own stdout rather than
// against a hand-written expectation, so this cannot pass by agreeing with a
// stale fixture.
//
// The public process path deliberately proves the live publication boundary
// after each response. Forced rotation and a short/erroring stdout sink are
// not injectable through Process.Execute or edges.Edges: production supplies
// the fixed default transcript policy and the caller owns the stdio writer.
// Those failure modes are therefore asserted in the owning wiretranscript and
// transport package tests rather than by constructing platform writers in a
// functional test.
func TestServeACPWritesAWireTranscriptByDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving you serve acp through root.BuildProcess")
	}

	home := t.TempDir()
	cwd := t.TempDir()
	// The transcript root is resolved from the real process environment at
	// connection time, the same way the Factory target catalog resolves its
	// home, so it must be set before the process graph is built.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	environment := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)

	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}

	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
	})

	var stderr bytes.Buffer
	support.StartProcessCommand(t, process, root.Input{
		Args:             []string{"you", "serve", "acp"},
		Env:              environment,
		Stdin:            stdinRead,
		Stdout:           stdoutWrite,
		Stderr:           &stderr,
		WorkingDirectory: cwd,
	})

	stdout := bufio.NewReader(stdoutRead)

	// One well-formed request and one deliberately malformed line: the
	// malformed line is the interesting case, because a rejected frame is
	// exactly what a customer opens the transcript to find.
	sent := []string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":%s}`, fixtureInitializeParams),
		`this line is not json`,
	}
	var received []string
	for _, line := range sent {
		writeRPCLine(t, stdinWrite, line)
		// Read the raw response line rather than a decoded frame: the point of
		// this cell is to compare the transcript against the actual bytes.
		raw, readErr := stdout.ReadString('\n')
		if readErr != nil {
			t.Fatalf("read response line: %v", readErr)
		}
		received = append(received, strings.TrimSpace(raw))
		assertOutboundTranscriptVisible(t, home, len(received))
	}
	_ = stdinWrite.Close()

	records := readWireTranscript(t, home)
	if len(records) == 0 {
		t.Fatal("no wire transcript records were written")
	}

	assertTranscriptMatchesTraffic(t, records, sent, received)
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

// readWireTranscript locates and decodes the single transcript this connection
// produced under home.
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
	if len(paths) != 1 {
		t.Fatalf("transcript files under %s = %v, want exactly one per connection", root, paths)
	}

	file, err := os.Open(paths[0])
	if err != nil {
		t.Fatalf("open transcript: %v", err)
	}
	defer func() { _ = file.Close() }()

	records, err := wiretranscript.ReadAll(file)
	if err != nil {
		t.Fatalf("decode transcript: %v", err)
	}
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

	var inbound, outbound []string
	var malformed int
	for _, record := range records {
		text := string(record.Frame)
		if record.Frame == nil {
			text = record.Text
		}
		if record.Err != "" {
			malformed++
		}
		switch record.Direction {
		case wiretranscript.DirectionIn:
			if record.Peer != wiretranscript.PeerClient {
				t.Fatalf("inbound record attributed to %q, want client", record.Peer)
			}
			inbound = append(inbound, text)
		case wiretranscript.DirectionOut:
			if record.Peer != wiretranscript.PeerAgent {
				t.Fatalf("outbound record attributed to %q, want agent", record.Peer)
			}
			outbound = append(outbound, text)
		}
	}

	assertSameJSONLines(t, "inbound", inbound, sent)
	assertSameJSONLines(t, "outbound", outbound, received)

	if malformed != 1 {
		t.Fatalf("records flagged malformed = %d, want exactly the one unparsable line", malformed)
	}
}

// assertSameJSONLines compares recorded lines against real ones, normalizing
// JSON so formatting differences do not masquerade as content differences
// while any change in content still fails.
func assertSameJSONLines(t *testing.T, direction string, recorded, actual []string) {
	t.Helper()
	if len(recorded) != len(actual) {
		t.Fatalf("%s records = %d, want %d\n recorded: %v\n actual: %v",
			direction, len(recorded), len(actual), recorded, actual)
	}
	for index := range recorded {
		if normalizeJSONLine(recorded[index]) != normalizeJSONLine(actual[index]) {
			t.Fatalf("%s record[%d] differs\n recorded %s\n actual   %s",
				direction, index, recorded[index], actual[index])
		}
	}
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
		t.Skip("integration test driving you serve acp through root.BuildProcess")
	}

	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	environment := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)

	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := process.Execute(root.Input{
		Args:             []string{"you", "serve", "acp"},
		Env:              environment,
		Stdin:            strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":%s}`, fixtureInitializeParams) + "\n"),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: cwd,
	}); err != nil {
		t.Fatalf("serve acp Execute() error = %v", err)
	}

	transcripts := wireTranscriptFiles(t, filepath.Join(home, ".you-agent-factory", "acp-wire"))
	if len(transcripts) != 1 {
		t.Fatalf("wire transcripts = %v, want exactly one for one connection", transcripts)
	}
	info, err := os.Stat(transcripts[0])
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", transcripts[0], err)
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

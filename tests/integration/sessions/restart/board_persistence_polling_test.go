package restart_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	// Structured terminal-log vocabulary shared by the whole-line JSON records
	// written to runtime logs and the Zap CONSOLE records written to stderr.
	boardPersistenceRunServiceOperation = "run.service"
	boardPersistenceOutcomeCancelled    = "cancelled"
	boardPersistenceCancelReason        = "context canceled"
	boardPersistencePlainCancelLine     = "Error: " + boardPersistenceCancelReason

	// Fixed positions inside a Zap CONSOLE record: time, level, caller, and
	// message precede the trailing structured context object.
	boardPersistenceConsoleTimeSegment   = 0
	boardPersistenceConsoleLevelSegment  = 1
	boardPersistenceConsoleCallerSegment = 2
	// A cancellation record carries four console prefixes plus the context.
	boardPersistenceConsoleContextSegments = 5
)

func newBoardPersistenceLogWatcher(t *testing.T, logDir string) *fsnotify.Watcher {
	t.Helper()
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("create isolated runtime log root: %v", err)
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("watch isolated runtime logs: %v", err)
	}
	if err := addBoardPersistenceLogDirectories(watcher, logDir); err != nil {
		_ = watcher.Close()
		t.Fatalf("watch isolated runtime log root %q: %v", logDir, err)
	}
	t.Cleanup(func() { _ = watcher.Close() })
	return watcher
}

func addBoardPersistenceLogDirectories(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
}

func waitForBoardStartupLogBeforeReadiness(
	t *testing.T,
	daemon *boardPersistenceDaemon,
	watcher *fsnotify.Watcher,
	timeout time.Duration,
) string {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		if path := boardPersistenceFirstLog(daemon.logDir); path != "" {
			if boardPersistenceDaemonReady(t, daemon) {
				t.Fatalf("successor reached public readiness before the parent observed runtime log write %q", path)
			}
			return path
		}
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				t.Fatal("runtime log watcher closed before startup barrier")
			}
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if err := addBoardPersistenceLogDirectories(watcher, event.Name); err != nil {
						t.Fatalf("watch new runtime log directory %q: %v", event.Name, err)
					}
				}
			}
			if event.Op&(fsnotify.Create|fsnotify.Write) != 0 && strings.HasSuffix(strings.ToLower(event.Name), ".log") {
				if boardPersistenceDaemonReady(t, daemon) {
					t.Fatalf("successor reached public readiness before the parent observed runtime log write %q", event.Name)
				}
				return event.Name
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				t.Fatal("runtime log watcher error stream closed before startup barrier")
			}
			t.Fatalf("observe runtime startup log: %v", err)
		case <-daemon.done:
			dumpBoardPersistenceDiagnostics(t, daemon)
			t.Fatalf("successor exited before observed startup barrier: %v", daemon.waitError())
		case <-deadline.C:
			t.Fatalf("timed out waiting for runtime log write before readiness in %q", daemon.logDir)
		}
	}
}

func boardPersistenceFirstLog(logDir string) string {
	var first string
	_ = filepath.WalkDir(logDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".log") {
			return nil
		}
		first = path
		return filepath.SkipAll
	})
	return first
}

func assertBoardResumeStartupWasCancelled(t *testing.T, daemon *boardPersistenceDaemon, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("successor startup log %q was not retained after exit: %v", path, err)
	}
	if !boardPersistenceCancellationObserved(daemon, path) {
		t.Fatalf("successor did not report cancellation before readiness (log=%q)", path)
	}
}

func boardPersistenceCancellationObserved(daemon *boardPersistenceDaemon, path string) bool {
	if daemon == nil {
		return false
	}
	if boardPersistenceReportsCancellation(daemon.stdout.String()) ||
		boardPersistenceReportsCancellation(daemon.stderr.String()) {
		return true
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return boardPersistenceReportsCancellation(string(contents))
}

func boardPersistenceReportsCancellation(logs string) bool {
	for _, line := range strings.Split(logs, "\n") {
		if boardPersistenceLineReportsCancellation(line) {
			return true
		}
	}
	return false
}

// boardPersistenceLineReportsCancellation classifies one captured output line.
// The terminal logger writes Zap CONSOLE records whose structured fields follow
// the message, so cancellation is read from either a whole-line JSON record or
// the console trailing context object. Both forms are classified from their
// fields alone; no message text is matched as a substring.
func boardPersistenceLineReportsCancellation(line string) bool {
	// The CLI currently reports an interrupted startup on stderr rather than
	// emitting a structured run.service record on this path.
	if strings.TrimSpace(line) == boardPersistencePlainCancelLine {
		return true
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(line), &fields); err == nil && boardPersistenceFieldsReportCancellation(fields) {
		return true
	}
	context, ok := boardPersistenceConsoleLogContext(line)
	if !ok {
		return false
	}
	return boardPersistenceFieldsReportCancellation(context)
}

// boardPersistenceConsoleLogContext parses the structured context of one Zap
// CONSOLE record written by the terminal logger:
//
//	<time>\t<level>\t<caller>\t<message>\t<json-context>
//
// The canonical timestamp, capital level, and source-location prefixes plus one
// complete JSON object context are all required, so ordinary terminal output,
// truncated records, and records that only mention cancellation never match.
func boardPersistenceConsoleLogContext(line string) (map[string]any, bool) {
	segments := strings.Split(strings.TrimSuffix(line, "\r"), "\t")
	if len(segments) < boardPersistenceConsoleContextSegments {
		return nil, false
	}
	if _, err := time.Parse(time.RFC3339Nano, segments[boardPersistenceConsoleTimeSegment]); err != nil {
		return nil, false
	}
	if !boardPersistenceConsoleLevel(segments[boardPersistenceConsoleLevelSegment]) {
		return nil, false
	}
	if !boardPersistenceConsoleCaller(segments[boardPersistenceConsoleCallerSegment]) {
		return nil, false
	}
	contextSegment := segments[len(segments)-1]
	if !strings.HasPrefix(contextSegment, "{") || !strings.HasSuffix(contextSegment, "}") {
		return nil, false
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(contextSegment), &fields); err != nil || fields == nil {
		return nil, false
	}
	return fields, true
}

// boardPersistenceConsoleLevel reports whether a segment is one of the capital
// level names the console encoder emits.
func boardPersistenceConsoleLevel(segment string) bool {
	switch segment {
	case "DEBUG", "INFO", "WARN", "ERROR", "DPANIC", "PANIC", "FATAL":
		return true
	default:
		return false
	}
}

// boardPersistenceConsoleCaller reports whether a segment is a short caller
// reference of the form "<path>/<file>.go:<line>".
func boardPersistenceConsoleCaller(segment string) bool {
	separator := strings.LastIndexByte(segment, ':')
	if separator <= 0 || separator == len(segment)-1 {
		return false
	}
	line, err := strconv.Atoi(segment[separator+1:])
	if err != nil || line <= 0 {
		return false
	}
	return strings.HasSuffix(segment[:separator], ".go")
}

// boardPersistenceFieldsReportCancellation classifies one decoded structured
// record from either encoding form.
func boardPersistenceFieldsReportCancellation(fields map[string]any) bool {
	if fields["operation"] == boardPersistenceRunServiceOperation && fields["outcome"] == boardPersistenceOutcomeCancelled {
		return true
	}
	// The runtime can stop after its first log write but before the
	// automation phase reports startup cancellation to the CLI.
	if fields["msg"] == "engine stopped" && fields["reason"] == boardPersistenceCancelReason {
		return true
	}
	if fields["level"] == "error" && fields["error"] == boardPersistenceCancelReason {
		message, _ := fields["msg"].(string)
		if message == "engine initial tick error" || message == "failed to compile factory orchestration" {
			return true
		}
	}
	return false
}

func TestBoardPersistenceReportsStructuredEngineCancellation(t *testing.T) {
	t.Parallel()
	if !boardPersistenceReportsCancellation(`{"level":"info","msg":"engine stopped","reason":"context canceled"}`) {
		t.Fatal("engine stop cancellation was not observed")
	}
	if boardPersistenceReportsCancellation(`{"level":"info","msg":"engine stopped","reason":"completed"}`) {
		t.Fatal("normal engine stop was classified as cancellation")
	}
}

// Console record segments shared by every classification case below. The
// terminal logger writes each record as
// "<time>\t<level>\t<caller>:<line>\t<message>\t<json-context>\n", and the
// timestamp and caller path are taken from the captured successor stderr line,
// so each case differs only in the segments under test.
const (
	boardPersistenceCapturedTime        = "2026-10-02T20:18:57.191Z"
	boardPersistenceCapturedCaller      = "run/invocation_observability.go"
	boardPersistenceCancellationContext = `{"operation": "run.service", "outcome": "cancelled", "hosting_intent": true, "failure_class": "none"}`
)

// capturedCancellationStderr is the exact 189-byte stderr line captured from the
// pre-readiness successor process in
// C:/t/dub-ci-e4dd-restart-diagnostics/cancellation-scenario.json (CI37059462705,
// head e4dd9b69), which exited 130 without a JSON record.
const capturedCancellationStderr = boardPersistenceCapturedTime + "\tINFO\t" +
	boardPersistenceCapturedCaller + ":89\trun service completed\t" +
	boardPersistenceCancellationContext + "\n"

// boardPersistenceConsoleRecord renders one Zap CONSOLE record from the captured
// prefix. An empty context omits the trailing object, which is how a record that
// stopped before structured logging is captured.
func boardPersistenceConsoleRecord(level string, callerLine int, message, context string) string {
	record := fmt.Sprintf("%s\t%s\t%s:%d\t%s", boardPersistenceCapturedTime, level, boardPersistenceCapturedCaller, callerLine, message)
	if context != "" {
		record += "\t" + context
	}
	return record + "\n"
}

type boardPersistenceCancellationCase struct {
	name string
	logs string
	want bool
}

// boardPersistenceConsoleCancellationCases returns the console-encoding cases,
// including the negatives that keep the parser from matching message text,
// incomplete prefixes, or other operations and outcomes.
func boardPersistenceConsoleCancellationCases() []boardPersistenceCancellationCase {
	return []boardPersistenceCancellationCase{
		{name: "captured console cancellation record", logs: capturedCancellationStderr, want: true},
		{name: "captured console cancellation record after unrelated startup output", logs: "Home directory: /tmp/successor-home\nRuntime log start (UTC): 2026-10-02 20:18:57 UTC\n" + capturedCancellationStderr, want: true},
		{name: "console success record", logs: boardPersistenceConsoleRecord("INFO", 141, "run service completed", `{"operation": "run.service", "outcome": "success", "hosting_intent": true, "failure_class": "none"}`), want: false},
		{name: "console failure record", logs: boardPersistenceConsoleRecord("ERROR", 128, "run service failed", `{"operation": "run.service", "outcome": "failure", "hosting_intent": true, "failure_class": "runtime_failure"}`), want: false},
		{name: "console cancellation record for another operation", logs: boardPersistenceConsoleRecord("INFO", 177, "run recovery outcome", `{"operation": "run.recovery", "outcome": "cancelled", "hosting_intent": true, "failure_class": "none"}`), want: false},
		{name: "console record whose message only mentions cancellation", logs: boardPersistenceConsoleRecord("INFO", 120, "invocation cancelled before readiness", `{"hosting_intent": true, "failure_class": "none"}`), want: false},
		{name: "console record without structured context", logs: boardPersistenceConsoleRecord("INFO", 89, "run service completed", ""), want: false},
		{name: "console record with truncated context", logs: boardPersistenceConsoleRecord("INFO", 89, "run service completed", `{"operation": "run.service", "outcome": "cancelled", "hosting_intent": true`), want: false},
		{name: "console record with unquoted context value", logs: boardPersistenceConsoleRecord("INFO", 89, "run service completed", `{"operation": "run.service", "outcome": cancelled}`), want: false},
		{name: "structured record without a console timestamp prefix", logs: "cancellation\tINFO\t" + boardPersistenceCapturedCaller + ":89\trun service completed\t" + boardPersistenceCancellationContext + "\n", want: false},
		{name: "structured record without a console caller segment", logs: boardPersistenceCapturedTime + "\tINFO\trun service completed\t" + boardPersistenceCancellationContext + "\n", want: false},
		{name: "plain line spelling cancellation differently", logs: "Error: context cancelled\n", want: false},
		{name: "preserved whole-line JSON cancellation record", logs: `{"level":"info","msg":"run service completed","operation":"run.service","outcome":"cancelled"}`, want: true},
		{name: "preserved plain cancellation line", logs: boardPersistencePlainCancelLine + "\n", want: true},
	}
}

// TestBoardPersistenceReportsConsoleRunServiceCancellation covers the terminal
// encoding of the run.service cancellation record that the pre-readiness
// successor writes to stderr, including capturedCancellationStderr itself.
func TestBoardPersistenceReportsConsoleRunServiceCancellation(t *testing.T) {
	t.Parallel()
	for _, testCase := range boardPersistenceConsoleCancellationCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := boardPersistenceReportsCancellation(testCase.logs); got != testCase.want {
				t.Fatalf("boardPersistenceReportsCancellation(%q) = %t, want %t", testCase.logs, got, testCase.want)
			}
		})
	}
}

func boardPersistenceDaemonReady(t *testing.T, daemon *boardPersistenceDaemon) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, daemon.baseURL+"/status", nil)
	if err != nil {
		t.Fatalf("build startup barrier readiness probe: %v", err)
	}
	response, err := (&http.Client{Timeout: 100 * time.Millisecond}).Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	var status factoryapi.StatusResponse
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&status) != nil {
		return false
	}
	return status.RuntimeStatus != ""
}

func waitForBoardDaemonReady(t *testing.T, daemon *boardPersistenceDaemon, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	// The real child process exposes no parent readiness channel. This bounded
	// public /status observation is the unavoidable process-bound synchronization
	// for the isolated-daemon acceptance test.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		select {
		case <-daemon.done:
			dumpBoardPersistenceDiagnostics(t, daemon)
			t.Fatalf("isolated you daemon exited before readiness: %v\nstdout=%s\nstderr=%s", daemon.waitError(), daemon.stdout.String(), daemon.stderr.String())
		case <-ticker.C:
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, daemon.baseURL+"/status", nil)
			if err != nil {
				t.Fatalf("build daemon readiness request: %v", err)
			}
			response, err := client.Do(request)
			if err != nil {
				continue
			}
			var status factoryapi.StatusResponse
			decodeErr := json.NewDecoder(response.Body).Decode(&status)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && status.RuntimeStatus != "" {
				daemon.readyAt = time.Now()
				return
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for isolated you daemon readiness at %s\nstdout=%s\nstderr=%s", daemon.baseURL, daemon.stdout.String(), daemon.stderr.String())
		}
	}
}

func dumpBoardPersistenceDiagnostics(t *testing.T, daemon *boardPersistenceDaemon) {
	t.Helper()
	t.Logf("daemon diagnostic paths: factory=%q home=%q record=%q", daemon.factoryDir, daemon.homeDir, daemon.recordPath)
	logsRoot := filepath.Join(daemon.homeDir, ".you-agent-factory", "logs")
	_ = filepath.WalkDir(logsRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".log") {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if len(contents) > 8192 {
			contents = contents[len(contents)-8192:]
		}
		t.Logf("daemon runtime log tail %q (%d bytes): %s", path, len(contents), contents)
		return nil
	})
}

func waitForBoardPersistenceSnapshot(t *testing.T, path, wantSessionID string, timeout time.Duration) []byte {
	t.Helper()
	// The durable snapshot is committed by the isolated daemon child, and the
	// parent has no synchronization channel for that filesystem write. Polling
	// the file is the only deterministic observation of the commit boundary;
	// the bounded timeout turns a failed child write into a useful test failure.
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		contents, err := os.ReadFile(path)
		if err == nil && len(contents) > 0 {
			var snapshot struct {
				Session struct {
					SessionID string `json:"sessionId"`
				} `json:"session"`
			}
			if err := json.Unmarshal(contents, &snapshot); err == nil && snapshot.Session.SessionID == wantSessionID {
				return contents
			}
			lastErr = fmt.Errorf("durable snapshot was empty or session identity was not %q", wantSessionID)
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for valid durable snapshot %q: %v", path, lastErr)
		}
	}
}

func waitForBoardPersistenceLogMessage(
	t *testing.T,
	daemon *boardPersistenceDaemon,
	fragments []string,
	timeout time.Duration,
) {
	t.Helper()
	// The recovery warning is appended by the isolated child process after boot,
	// with no test-owned logging edge back to the parent. Polling the runtime log
	// is therefore the required process-boundary observation; the timeout keeps
	// a missing warning actionable without using a fixed sleep.
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		found := false
		_ = filepath.WalkDir(daemon.logDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".log") {
				return nil
			}
			contents, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			for _, fragment := range fragments {
				if !strings.Contains(string(contents), fragment) {
					return nil
				}
			}
			found = true
			return filepath.SkipAll
		})
		if found {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for recovery warning in runtime logs under %q", daemon.logDir)
		}
	}
}

func waitForBoardStates(t *testing.T, baseURL string, want map[string]string, timeout time.Duration) factoryapi.ListWorkResponse {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	// Work has no process-parent notification, so state convergence is observed
	// through the public session list with a bounded ticker rather than a fixed
	// sleep. This is the process-bound wait required by this test only.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last factoryapi.ListWorkResponse
	var lastErr error
	for {
		listed, err := readBoardWorkList(t.Context(), baseURL)
		if err == nil {
			last = listed
			if boardStatesMatch(listed, want) {
				return listed
			}
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for Work states %#v; last list=%#v, last error=%v", want, last.Results, lastErr)
		}
	}
}

func readBoardWorkList(ctx context.Context, baseURL string) (factoryapi.ListWorkResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+"/factory-sessions/~default/work", nil)
	if err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return factoryapi.ListWorkResponse{}, fmt.Errorf("GET /work status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var listed factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	return listed, nil
}

func boardStatesMatch(listed factoryapi.ListWorkResponse, want map[string]string) bool {
	if len(listed.Results) != len(want) {
		return false
	}
	seen := make(map[string]struct{}, len(listed.Results))
	for _, item := range listed.Results {
		workID := boardPersistenceStringPointerValue(item.WorkId)
		state := ""
		if item.State != nil {
			state = item.State.Name
		}
		if _, duplicate := seen[workID]; duplicate || want[workID] != state {
			return false
		}
		seen[workID] = struct{}{}
	}
	return len(seen) == len(want)
}

package cancel_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	cancelArtifactEnvironment         = "INFINITE_YOU_INTEGRATION_BINARY"
	cancelArtifactFallbackEnvironment = "INFINITE_YOU_PREBUILT_ARTIFACT"
	cancelArtifactRequiredEnvironment = "INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT"
	cancelEvidenceEnvironment         = "FACTORY_RELIABILITY_CANCEL_EVIDENCE_PATH"
	cancelGitHeadEnvironment          = "FACTORY_RELIABILITY_CANCEL_GIT_HEAD"
	cancelFactorySessionTimeout       = 10 * time.Minute
	cancelProcessBound                = 10 * time.Second
	cancelProcessSampleInterval       = time.Second
	cancelFixtureResourceName         = "agent-slot"
	cancelFixtureLateOutput           = "cancel-child-late-output"
)

type cancelDaemon struct {
	cmd     *exec.Cmd
	done    chan struct{}
	mu      sync.Mutex
	waitErr error
	stopped bool
	stdout  *cancelSafeBuffer
	stderr  *cancelSafeBuffer
}

type cancelSafeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (buffer *cancelSafeBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buf.Write(data)
}

func (buffer *cancelSafeBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buf.String()
}

type cancelCommandResult struct {
	stdout string
	stderr string
	err    error
}

type workerProcessTree struct {
	WorkID   string
	RootPID  int
	ChildPID int
	PIDs     []int
}

type cancelProcessSample struct {
	AfterAPIMillis       int64 `json:"afterApiMillis"`
	TargetPIDs           []int `json:"targetPids"`
	TargetPresent        []int `json:"targetPresent"`
	TargetDescendants    []int `json:"targetDescendants"`
	UnrelatedPIDs        []int `json:"unrelatedPids"`
	UnrelatedPresent     []int `json:"unrelatedPresent"`
	UnrelatedDescendants []int `json:"unrelatedDescendants"`
}

type cancelCleanupCensus struct {
	TargetPIDsRemaining    []int `json:"targetPidsRemaining"`
	UnrelatedPIDsRemaining []int `json:"unrelatedPidsRemaining"`
	DaemonPIDRemaining     bool  `json:"daemonPidRemaining"`
	ListenerAvailable      bool  `json:"listenerAvailable"`
}

type cancelEvidenceManifest struct {
	Schema                          string                `json:"schema"`
	Outcome                         string                `json:"outcome"`
	ArtifactPath                    string                `json:"artifactPath"`
	BinarySHA256                    string                `json:"binarySha256"`
	FixtureSHA256                   string                `json:"fixtureSha256"`
	GitHead                         string                `json:"gitHead,omitempty"`
	GoVersion                       string                `json:"goVersion"`
	OS                              string                `json:"os"`
	Arch                            string                `json:"arch"`
	Listener                        string                `json:"listener"`
	FactorySessionID                string                `json:"factorySessionId"`
	TargetWorkID                    string                `json:"targetWorkId"`
	UnrelatedWorkID                 string                `json:"unrelatedWorkId"`
	TargetWorkerSessionID           string                `json:"targetWorkerSessionId"`
	UnrelatedWorkerSessionID        string                `json:"unrelatedWorkerSessionId"`
	TargetDispatchID                string                `json:"targetDispatchId"`
	APIStatus                       int                   `json:"apiStatus"`
	APIControlStartedAt             time.Time             `json:"apiControlStartedAt"`
	APIAppliedAt                    time.Time             `json:"apiAppliedAt"`
	APIOutcome                      string                `json:"apiOutcome"`
	APIState                        string                `json:"apiState"`
	DispatchResponseAt              time.Time             `json:"dispatchResponseAt"`
	CommandCompletionAfterAPIMillis int64                 `json:"commandCompletionAfterApiMillis"`
	CLIRepeatExit                   int                   `json:"cliRepeatExit"`
	CLIRepeatOutcome                string                `json:"cliRepeatOutcome"`
	CLIRepeatState                  string                `json:"cliRepeatState"`
	AuthorizedSuccessorCount        int                   `json:"authorizedSuccessorCount"`
	TargetTreeBefore                []int                 `json:"targetTreeBefore"`
	TargetSuccessorTreeBefore       []int                 `json:"targetSuccessorTreeBefore"`
	UnrelatedTreeBefore             []int                 `json:"unrelatedTreeBefore"`
	SampleInterval                  string                `json:"sampleInterval"`
	ProcessBound                    string                `json:"processBound"`
	Samples                         []cancelProcessSample `json:"samples"`
	TargetGoneAfterAPIMillis        int64                 `json:"targetGoneAfterApiMillis"`
	TargetFailureKind               string                `json:"targetFailureKind"`
	TargetTranscript                string                `json:"targetTranscript"`
	WorkerTerminalCount             int                   `json:"workerTerminalCount"`
	WorkerTerminalPhase             string                `json:"workerTerminalPhase"`
	ScriptResponseCount             int                   `json:"scriptResponseCount"`
	ScriptResponseOutcome           string                `json:"scriptResponseOutcome"`
	LateOutputRetained              bool                  `json:"lateOutputRetained"`
	EventIDsBeforeRepeat            []string              `json:"eventIdsBeforeRepeat"`
	EventIDsAfterRepeat             []string              `json:"eventIdsAfterRepeat"`
	CanceledDispatchRequestCount    int                   `json:"canceledDispatchRequestCount"`
	CanceledDispatchResponseCount   int                   `json:"canceledDispatchResponseCount"`
	CancellationReason              string                `json:"cancellationReason"`
	ResourceReleaseCount            int                   `json:"resourceReleaseCount"`
	CanceledWorkState               string                `json:"canceledWorkState"`
	UnrelatedWorkStateAfter         string                `json:"unrelatedWorkStateAfter"`
	UnrelatedSessionStateAfter      string                `json:"unrelatedSessionStateAfter"`
	UnknownAPIStatus                int                   `json:"unknownApiStatus"`
	UnknownAPICode                  string                `json:"unknownApiCode"`
	UnknownCLIExit                  int                   `json:"unknownCliExit"`
	UnknownCLICode                  string                `json:"unknownCliCode"`
	CleanupMethod                   string                `json:"cleanupMethod"`
	CleanupCensus                   cancelCleanupCensus   `json:"cleanupCensus"`
}

type canceledDispatchEvidence struct {
	Requests           int
	Responses          int
	ResourceReleases   int
	CancellationReason string
	OutputWorkState    string
	LateOutputReturned bool
}

func resolveCancelArtifact(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(cancelArtifactEnvironment))
	if path == "" {
		path = strings.TrimSpace(os.Getenv(cancelArtifactFallbackEnvironment))
	}
	if path == "" {
		if strings.EqualFold(strings.TrimSpace(os.Getenv(cancelArtifactRequiredEnvironment)), "1") {
			t.Fatalf("%s or %s is required; this integration test never builds the CLI", cancelArtifactEnvironment, cancelArtifactFallbackEnvironment)
		}
		t.Skip("prebuilt integration artifact unavailable; set INFINITE_YOU_INTEGRATION_BINARY")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve prebuilt integration artifact %q: %v", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		t.Fatalf("stat prebuilt integration artifact %q: %v", abs, err)
	}
	if info.IsDir() || info.Size() == 0 {
		t.Fatalf("prebuilt integration artifact %q is not a non-empty file", abs)
	}
	return abs
}

func startCancelDaemon(t *testing.T, ctx context.Context, binaryPath string, fixture cancelFixture) *cancelDaemon {
	t.Helper()
	command := exec.CommandContext(ctx, binaryPath,
		"run", "--dir", fixture.factoryDir, "--continuously", "--with-server",
		"--listen", net.JoinHostPort("127.0.0.1", strconv.Itoa(fixture.port)),
		"--record", fixture.recordPath,
	)
	command.Dir = fixture.factoryDir
	command.Env = append([]string(nil), fixture.environment...)
	stdout, stderr := &cancelSafeBuffer{}, &cancelSafeBuffer{}
	command.Stdout, command.Stderr = stdout, stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start prebuilt Factory daemon: %v", err)
	}
	daemon := &cancelDaemon{cmd: command, done: make(chan struct{}), stdout: stdout, stderr: stderr}
	go func() {
		err := command.Wait()
		daemon.mu.Lock()
		daemon.waitErr = err
		daemon.mu.Unlock()
		close(daemon.done)
	}()
	t.Cleanup(func() { cleanupCancelDaemon(daemon) })
	return daemon
}

func waitForCancelFactorySession(t *testing.T, ctx context.Context, serverURL string, daemon *cancelDaemon) string {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	for {
		status, err := getJSON[factoryapi.StatusResponse](ctx, client, serverURL+"/status")
		if err == nil && status.FactoryState == "RUNNING" {
			listed, listErr := getJSON[factoryapi.ListFactorySessionsResponse](ctx, client, serverURL+"/factory-sessions?scope=live")
			if listErr == nil {
				for _, session := range listed.Sessions {
					if session.IsDefault && strings.TrimSpace(session.Id) != "" {
						return session.Id
					}
				}
				lastErr = errors.New("live Factory Session list has no default session")
			} else {
				lastErr = listErr
			}
		} else if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("factory state=%q runtime=%q", status.FactoryState, status.RuntimeStatus)
		}
		select {
		case <-daemon.done:
			t.Fatalf("prebuilt Factory daemon exited before readiness: %v; stdout=%s stderr=%s", daemon.waitError(), daemon.stdout.String(), daemon.stderr.String())
		case <-ctx.Done():
			t.Fatalf("wait for compiled Factory Session: %v (last error: %v)", ctx.Err(), lastErr)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func submitCancelWork(t *testing.T, ctx context.Context, serverURL, sessionID, name string) string {
	t.Helper()
	requestBody, err := json.Marshal(factoryapi.SubmitWorkRequest{
		Name: stringPointer(name), WorkTypeName: "task", Payload: map[string]string{"controlFixture": name},
	})
	if err != nil {
		t.Fatalf("marshal Work request %q: %v", name, err)
	}
	endpoint := sessionWorkURL(serverURL, sessionID)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("create Work request %q: %v", name, err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("submit Work %q: %v", name, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read Work %q response: %v", name, err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("submit Work %q returned HTTP %d: %s", name, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var submitted factoryapi.SubmitWorkResponse
	if err := json.Unmarshal(body, &submitted); err != nil || submitted.WorkId == nil || strings.TrimSpace(*submitted.WorkId) == "" {
		t.Fatalf("decode Work %q response: %v; response=%s", name, err, strings.TrimSpace(string(body)))
	}
	return *submitted.WorkId
}

func waitForRunningWorkerSession(t *testing.T, ctx context.Context, serverURL, sessionID, workID string, daemon *cancelDaemon) factoryapi.WorkerSessionObservation {
	t.Helper()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	var last []factoryapi.WorkerSessionObservation
	var lastErr error
	for {
		listed, err := readWorkerSessionsValue(ctx, serverURL, sessionID, workID)
		if err == nil {
			last = listed.Sessions
			for _, session := range listed.Sessions {
				if session.State == factoryapi.WorkerSessionObservationStateRunning && session.WorkId != nil && *session.WorkId == workID {
					return session
				}
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			board, boardErr := readCancelWorkBoardDiagnostic(serverURL, sessionID, workID)
			var work *factoryapi.Work
			for index := range board.Results {
				if board.Results[index].WorkId != nil && *board.Results[index].WorkId == workID {
					work = &board.Results[index]
					break
				}
			}
			stdout, stderr := cancelDaemonOutput(daemon)
			t.Fatalf("wait for running Worker Session for Work %q: %v; sessions=%#v error=%v Work=%s boardError=%v daemonExited=%v status=%s topology=%v events=%v stdout=%s stderr=%s", workID, ctx.Err(), last, lastErr, workJSON(work), boardErr, channelClosed(daemon.done), cancelDaemonStatusDiagnostic(serverURL), cancelTopologyDiagnostic(serverURL, sessionID), cancelDaemonEventDiagnostic(serverURL, sessionID), stdout, stderr)
		case <-deadline.C:
			board, boardErr := readCancelWorkBoardDiagnostic(serverURL, sessionID, workID)
			var work *factoryapi.Work
			for index := range board.Results {
				if board.Results[index].WorkId != nil && *board.Results[index].WorkId == workID {
					work = &board.Results[index]
					break
				}
			}
			stdout, stderr := cancelDaemonOutput(daemon)
			t.Fatalf("wait for running Worker Session for Work %q timed out; sessions=%#v error=%v Work=%s boardError=%v daemonExited=%v status=%s topology=%v events=%v stdout=%s stderr=%s", workID, last, lastErr, workJSON(work), boardErr, channelClosed(daemon.done), cancelDaemonStatusDiagnostic(serverURL), cancelTopologyDiagnostic(serverURL, sessionID), cancelDaemonEventDiagnostic(serverURL, sessionID), stdout, stderr)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func workJSON(work *factoryapi.Work) string {
	if work == nil {
		return "<missing>"
	}
	encoded, err := json.Marshal(work)
	if err != nil {
		return fmt.Sprintf("<encode error: %v>", err)
	}
	return string(encoded)
}

func cancelDaemonStatusDiagnostic(serverURL string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	status, err := getJSON[factoryapi.StatusResponse](ctx, http.DefaultClient, strings.TrimSuffix(serverURL, "/")+"/status")
	if err != nil {
		return fmt.Sprintf("<status error: %v>", err)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		return fmt.Sprintf("<status encode error: %v>", err)
	}
	return string(encoded)
}

func cancelDaemonEventDiagnostic(serverURL, sessionID string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := readFactoryEvents(ctx, serverURL, sessionID)
	if err != nil {
		return []string{"<events error: " + err.Error() + ">"}
	}
	summary := make([]string, 0, len(events))
	for _, event := range events {
		dispatchID := ""
		if event.Context.DispatchId != nil {
			dispatchID = *event.Context.DispatchId
		}
		summary = append(summary, fmt.Sprintf("%s:%s:%s", event.Type, event.Id, dispatchID))
	}
	return summary
}

func cancelTopologyDiagnostic(serverURL, sessionID string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := readFactoryEvents(ctx, serverURL, sessionID)
	if err != nil {
		return []string{"<events error: " + err.Error() + ">"}
	}
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInitialStructureRequest {
			continue
		}
		payload, err := event.Payload.AsInitialStructureRequestEventPayload()
		if err != nil {
			return []string{"<initial structure decode error: " + err.Error() + ">"}
		}
		if payload.Factory.Workstations == nil {
			return []string{"<factory has no workstations>"}
		}
		var summary []string
		for _, workstation := range *payload.Factory.Workstations {
			encoded, err := json.Marshal(workstation)
			if err != nil {
				return []string{"<workstation encode error: " + err.Error() + ">"}
			}
			summary = append(summary, string(encoded))
		}
		return summary
	}
	return []string{"<initial structure event missing>"}
}

func readCancelWorkBoardDiagnostic(serverURL, sessionID, workID string) (factoryapi.ListWorkResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return getJSON[factoryapi.ListWorkResponse](ctx, http.DefaultClient, sessionWorkURL(serverURL, sessionID))
}

func cancelDaemonOutput(daemon *cancelDaemon) (string, string) {
	if daemon == nil || !channelClosed(daemon.done) {
		return "<daemon still running>", "<daemon still running>"
	}
	return daemon.stdout.String(), daemon.stderr.String()
}

func channelClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func readWorkerSessions(t *testing.T, ctx context.Context, serverURL, sessionID, workID string) factoryapi.ListWorkerSessionsResponse {
	t.Helper()
	listed, err := readWorkerSessionsValue(ctx, serverURL, sessionID, workID)
	if err != nil {
		t.Fatalf("list Worker Sessions for Work %q: %v", workID, err)
	}
	return listed
}

func readWorkerSessionsValue(ctx context.Context, serverURL, sessionID, workID string) (factoryapi.ListWorkerSessionsResponse, error) {
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/worker-sessions?workId=" + url.QueryEscape(workID)
	return getJSON[factoryapi.ListWorkerSessionsResponse](ctx, http.DefaultClient, endpoint)
}

func readWorkerSessionDetail(t *testing.T, ctx context.Context, serverURL, sessionID, workerSessionID string) factoryapi.WorkerSessionObservation {
	t.Helper()
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/worker-sessions/" + url.PathEscape(workerSessionID)
	result, err := getJSON[factoryapi.WorkerSessionObservation](ctx, http.DefaultClient, endpoint)
	if err != nil {
		t.Fatalf("read Worker Session detail %q: %v", workerSessionID, err)
	}
	return result
}

func readCancelWork(t *testing.T, ctx context.Context, serverURL, sessionID, workID string) factoryapi.Work {
	t.Helper()
	endpoint := sessionWorkURL(serverURL, sessionID)
	listed, err := getJSON[factoryapi.ListWorkResponse](ctx, http.DefaultClient, endpoint)
	if err != nil {
		t.Fatalf("read Work board for %q: %v", workID, err)
	}
	for _, work := range listed.Results {
		if work.WorkId != nil && *work.WorkId == workID {
			return work
		}
	}
	t.Fatalf("Work board omitted exact Work %q: %#v", workID, listed.Results)
	return factoryapi.Work{}
}

func sessionWorkURL(serverURL, sessionID string) string {
	return strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
}

func postWorkerSessionCancel(ctx context.Context, serverURL, workerSessionID string) (int, []byte, error) {
	endpoint := strings.TrimSuffix(serverURL, "/") + "/worker-sessions/" + url.PathEscape(workerSessionID) + "/cancel"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func runCancelCLI(ctx context.Context, binaryPath string, fixture cancelFixture, args ...string) cancelCommandResult {
	arguments := append([]string{"--remote", "--server", fixture.serverURL, "--json"}, args...)
	command := exec.CommandContext(ctx, binaryPath, arguments...)
	command.Dir = fixture.factoryDir
	command.Env = append([]string(nil), fixture.environment...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return cancelCommandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func readFactoryEvents(ctx context.Context, serverURL, sessionID string) ([]factoryapi.FactoryEvent, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/events"
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("GET Factory Events returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	retainedCount, err := strconv.Atoi(strings.TrimSpace(response.Header.Get(factorysessions.SessionEventStreamRetainedCountHeader)))
	if err != nil {
		return nil, fmt.Errorf("parse retained Factory Event count %q: %w", response.Header.Get(factorysessions.SessionEventStreamRetainedCountHeader), err)
	}
	if retainedCount < 0 {
		return nil, fmt.Errorf("retained Factory Event count %d is negative", retainedCount)
	}
	events := make([]factoryapi.FactoryEvent, 0, retainedCount)
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for len(events) < retainedCount && scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			return events, fmt.Errorf("decode Factory Event: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return events, err
	}
	if len(events) != retainedCount {
		return events, fmt.Errorf("Factory Event replay ended after %d of %d retained events", len(events), retainedCount)
	}
	return events, nil
}

func waitForCanceledDispatchEvent(t *testing.T, ctx context.Context, serverURL, sessionID, dispatchID string) error {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		events, err := readFactoryEvents(ctx, serverURL, sessionID)
		if err == nil {
			for _, event := range events {
				if event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID || event.Type != factoryapi.FactoryEventTypeDispatchResponse {
					continue
				}
				payload, decodeErr := event.Payload.AsDispatchResponseEventPayload()
				if decodeErr != nil {
					return decodeErr
				}
				if payload.Cancellation != nil && string(payload.Cancellation.Reason) == "CANCELED" && payload.Outcome == factoryapi.WorkOutcomeCanceled {
					return nil
				}
				return fmt.Errorf("dispatch %q response outcome=%q cancellation=%#v", dispatchID, payload.Outcome, payload.Cancellation)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for dispatch %q response (last event read error %v)", dispatchID, err)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func dispatchResponseTime(events []factoryapi.FactoryEvent, dispatchID string) (time.Time, error) {
	var responseTime time.Time
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse || event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID {
			continue
		}
		if !responseTime.IsZero() {
			return time.Time{}, fmt.Errorf("dispatch %q has more than one response event", dispatchID)
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			return time.Time{}, err
		}
		if payload.Cancellation == nil || string(payload.Cancellation.Reason) != "CANCELED" || payload.Outcome != factoryapi.WorkOutcomeCanceled {
			return time.Time{}, fmt.Errorf("dispatch %q response does not record canceled command completion: %#v", dispatchID, payload)
		}
		responseTime = event.Context.EventTime
	}
	if responseTime.IsZero() {
		return time.Time{}, fmt.Errorf("dispatch %q has no response event", dispatchID)
	}
	return responseTime, nil
}

func readWorkerSessionEvents(ctx context.Context, serverURL, sessionID, workerSessionID string) ([]factoryapi.WorkerSessionEvent, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) +
		"/worker-sessions/" + url.PathEscape(workerSessionID) + "/events?replayOnly=true"
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("GET Worker Session Events returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var frames []factoryapi.WorkerSessionEvent
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 32*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var frame factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &frame); err != nil {
			return frames, fmt.Errorf("decode Worker Session Event: %w", err)
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		return frames, err
	}
	if len(frames) == 0 || frames[len(frames)-1].ReplaySummary == nil {
		return frames, errors.New("Worker Session retained replay omitted its completeness summary")
	}
	return frames, nil
}

func terminalWorkerEventFacts(frames []factoryapi.WorkerSessionEvent) (int, string) {
	count, phase := 0, ""
	for _, frame := range frames {
		if frame.Event.SchemaId != "DISPATCH_RESPONSE" {
			continue
		}
		value, _ := frame.Event.Payload["outcome"].(string)
		cancellation, _ := frame.Event.Payload["cancellation"].(map[string]interface{})
		reason, _ := cancellation["reason"].(string)
		if value == "CANCELED" && reason == "CANCELED" {
			count++
			phase = value
		}
	}
	return count, phase
}

func scriptResponseFacts(frames []factoryapi.WorkerSessionEvent) (int, string, string) {
	count, outcome, stdout := 0, "", ""
	for _, frame := range frames {
		if frame.Event.SchemaId != "SCRIPT_RESPONSE" {
			continue
		}
		count++
		outcome, _ = frame.Event.Payload["outcome"].(string)
		stdout, _ = frame.Event.Payload["stdout"].(string)
	}
	return count, outcome, stdout
}

func cancelSuccessorObservation(
	t *testing.T,
	listed factoryapi.ListWorkerSessionsResponse,
	canceledWorkerSessionID, workID string,
) factoryapi.WorkerSessionObservation {
	t.Helper()
	for _, observation := range listed.Sessions {
		if observation.WorkerSessionId == canceledWorkerSessionID {
			continue
		}
		if observation.WorkId == nil || *observation.WorkId != workID ||
			observation.State != factoryapi.WorkerSessionObservationStateRunning || observation.Failure != nil {
			t.Fatalf("authorized successor for canceled Work %q = %#v, want one RUNNING session for the same Work", workID, observation)
		}
		return observation
	}
	t.Fatalf("target Work %q had no Worker Session other than canceled session %q", workID, canceledWorkerSessionID)
	return factoryapi.WorkerSessionObservation{}
}

func assertCanceledWorkerObservation(observation factoryapi.WorkerSessionObservation, workerSessionID, workID, dispatchID string) error {
	if observation.WorkerSessionId != workerSessionID || observation.State != factoryapi.WorkerSessionObservationStateCanceled {
		return fmt.Errorf("identity/state=%q/%q, want %q/CANCELED", observation.WorkerSessionId, observation.State, workerSessionID)
	}
	if observation.AttemptId != dispatchID || observation.WorkId == nil || *observation.WorkId != workID {
		return fmt.Errorf("attempt/Work=%q/%v, want %q/%q", observation.AttemptId, observation.WorkId, dispatchID, workID)
	}
	if observation.Failure == nil || observation.Failure.Kind != "OPERATOR_CANCELED" {
		return fmt.Errorf("failure=%#v, want OPERATOR_CANCELED", observation.Failure)
	}
	if observation.Transcript != factoryapi.WorkerSessionObservationTranscriptUNAVAILABLE {
		return fmt.Errorf("transcript=%q, want explicit UNAVAILABLE for script Worker Session", observation.Transcript)
	}
	return nil
}

func assertTranscriptUnavailable(ctx context.Context, serverURL, sessionID, workerSessionID string) error {
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) +
		"/worker-sessions/" + url.PathEscape(workerSessionID) + "/transcript"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusInternalServerError || errorResponseCode(body) != string(factoryapi.ErrorResponseCodeWORKERSESSIONTRANSCRIPTUNAVAILABLE) {
		return fmt.Errorf("transcript read returned HTTP %d code %q; want typed WORKER_SESSION_TRANSCRIPT_UNAVAILABLE", response.StatusCode, errorResponseCode(body))
	}
	return nil
}

func canceledDispatchFacts(events []factoryapi.FactoryEvent, workID, dispatchID string) (canceledDispatchEvidence, error) {
	var facts canceledDispatchEvidence
	for _, event := range events {
		if event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID || !eventHasWorkID(event, workID) {
			continue
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			facts.Requests++
		case factoryapi.FactoryEventTypeDispatchResponse:
			facts.Responses++
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				return facts, err
			}
			if payload.Cancellation != nil {
				facts.CancellationReason = string(payload.Cancellation.Reason)
			}
			if payload.Output != nil && strings.Contains(*payload.Output, cancelFixtureLateOutput) {
				facts.LateOutputReturned = true
			}
			if payload.OutputResources != nil {
				for _, resource := range *payload.OutputResources {
					if resource.Name == cancelFixtureResourceName && resource.Capacity == 2 {
						facts.ResourceReleases++
					}
				}
			}
			if payload.OutputWork != nil {
				for _, work := range *payload.OutputWork {
					if work.WorkId != nil && *work.WorkId == workID && work.State != nil {
						facts.OutputWorkState = work.State.Name
					}
				}
			}
		}
	}
	if facts.Requests != 1 || facts.Responses != 1 || facts.CancellationReason != "CANCELED" {
		return facts, fmt.Errorf("dispatch requests/responses/cancellation=%d/%d/%q", facts.Requests, facts.Responses, facts.CancellationReason)
	}
	return facts, nil
}

func eventHasWorkID(event factoryapi.FactoryEvent, workID string) bool {
	if event.Context.WorkIds == nil {
		return false
	}
	for _, id := range *event.Context.WorkIds {
		if id == workID {
			return true
		}
	}
	return false
}

func factoryEventIDs(events []factoryapi.FactoryEvent) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.Id)
	}
	return ids
}

func observationByID(t *testing.T, listed factoryapi.ListWorkerSessionsResponse, workerSessionID string) factoryapi.WorkerSessionObservation {
	t.Helper()
	for _, observation := range listed.Sessions {
		if observation.WorkerSessionId == workerSessionID {
			return observation
		}
	}
	t.Fatalf("Worker Session list omitted %q: %#v", workerSessionID, listed.Sessions)
	return factoryapi.WorkerSessionObservation{}
}

func workerSessionIDs(listed factoryapi.ListWorkerSessionsResponse) []string {
	ids := make([]string, 0, len(listed.Sessions))
	for _, observation := range listed.Sessions {
		ids = append(ids, observation.WorkerSessionId)
	}
	sort.Strings(ids)
	return ids
}

func failureKind(observation factoryapi.WorkerSessionObservation) string {
	if observation.Failure == nil {
		return ""
	}
	return observation.Failure.Kind
}

func sameActiveWorkerObservation(before, after factoryapi.WorkerSessionObservation) bool {
	return before.WorkerSessionId == after.WorkerSessionId && before.AttemptId == after.AttemptId &&
		before.State == factoryapi.WorkerSessionObservationStateRunning && after.State == factoryapi.WorkerSessionObservationStateRunning &&
		before.WorkId != nil && after.WorkId != nil && *before.WorkId == *after.WorkId &&
		reflect.DeepEqual(before.WorkIds, after.WorkIds) && after.Failure == nil
}

func sameWorkStateAndContent(before, after factoryapi.Work) bool {
	return before.WorkId != nil && after.WorkId != nil && *before.WorkId == *after.WorkId &&
		reflect.DeepEqual(before.State, after.State) && reflect.DeepEqual(before.Content, after.Content)
}

func workStateName(work factoryapi.Work) string {
	if work.State == nil {
		return ""
	}
	return work.State.Name
}

func workContentText(work factoryapi.Work) string {
	if work.Content == nil {
		return ""
	}
	encoded, _ := json.Marshal(work.Content)
	return string(encoded)
}

func getJSON[T any](ctx context.Context, client *http.Client, endpoint string) (T, error) {
	var result T
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result, err
	}
	response, err := client.Do(request)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return result, fmt.Errorf("GET %s returned HTTP %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return result, err
	}
	return result, nil
}

func errorResponseCode(body []byte) string {
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return ""
	}
	return string(response.Code)
}

func firstErrorCode(outputs ...string) string {
	for _, output := range outputs {
		if code := errorResponseCode([]byte(strings.TrimSpace(output))); code != "" {
			return code
		}
	}
	return ""
}

func stringPointer(value string) *string { return &value }

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func saveCancelEvidence(t *testing.T, manifest cancelEvidenceManifest) {
	path := strings.TrimSpace(os.Getenv(cancelEvidenceEnvironment))
	if path == "" {
		encoded, err := json.Marshal(manifest)
		if err == nil {
			t.Logf("cancel evidence manifest: %s", encoded)
		}
		return
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Logf("encode cancel evidence manifest: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Logf("create cancel evidence directory: %v", err)
		return
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Logf("write cancel evidence manifest %q: %v", path, err)
		return
	}
	t.Logf("cancel evidence manifest: %s", path)
}

package cli_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const (
	workerSessionsCodexSuccessID = "session_fixture_codex_success"
	workerSessionsCodexFailureID = "session_fixture_codex_structured_failure"
)

// TestWorkerSessionsCLI proves the complete diagnosis path through one
// root-built application: accepted Work is executed through an injected
// provider command runner, and public CLI list/show/stream/read commands
// correlate successful and failed provider activity back to Work and attempt
// identity, timing, usage, transcript, and failure cause.
func TestWorkerSessionsCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	caseFixture := newWorkerSessionsCLICase(t)
	fixture := caseFixture.fixture
	process := fixture.process
	factoryDir := caseFixture.factoryDir
	env := functionalEnvironment(fixture.homeDir)
	baseURL := fixture.baseURL
	caseFixture.registerRoutes(t, "worker-session-cli-success")
	t.Run("FT-B04 duplicate route registration fails closed", func(t *testing.T) {
		routeCount := fixture.runner.routeCount()
		callCount := fixture.runner.CallCount()
		if _, err := fixture.runner.registerRoute("worker-session-cli-success"); err == nil {
			t.Fatal("duplicate provider fixture route registration returned nil error")
		} else if !strings.Contains(err.Error(), "already registered") {
			t.Fatalf("duplicate provider fixture route error = %v, want already registered diagnostic", err)
		}
		if got := fixture.runner.routeCount(); got != routeCount {
			t.Fatalf("provider fixture route count after duplicate registration = %d, want %d", got, routeCount)
		}
		if got := fixture.runner.CallCount(); got != callCount {
			t.Fatalf("provider fixture call count after duplicate registration = %d, want %d", got, callCount)
		}
		if len(caseFixture.sessionIDs) != 0 {
			t.Fatalf("duplicate route registration opened Factory Sessions: %#v", caseFixture.sessionIDs)
		}
		assertFactorySessionFolderAbsent(t, baseURL, factoryDir)
	})

	routeStart := fixture.runner.CallCount()
	successFactorySessionID := caseFixture.openSession(t)
	failureFactorySessionID := caseFixture.openSession(t)
	emptyFactorySessionID := caseFixture.openSession(t)

	t.Run("FT-H01 help and completion do not mutate state", func(t *testing.T) {
		before := captureWorkerSessionsCLIPublicState(t, fixture, successFactorySessionID, failureFactorySessionID)
		assertWorkerSessionsCLIHelp(t, ctx, process, env, factoryDir)
		after := captureWorkerSessionsCLIPublicState(t, fixture, successFactorySessionID, failureFactorySessionID)
		assertWorkerSessionsCLIPublicStateUnchanged(t, before, after)
	})
	t.Run("FT-U01 unknown input does not mutate state", func(t *testing.T) {
		before := captureWorkerSessionsCLIPublicState(t, fixture, successFactorySessionID, failureFactorySessionID)
		assertWorkerSessionsCLIUnknown(t, ctx, process, env, factoryDir)
		after := captureWorkerSessionsCLIPublicState(t, fixture, successFactorySessionID, failureFactorySessionID)
		assertWorkerSessionsCLIPublicStateUnchanged(t, before, after)
	})
	t.Run("FT-B01 empty explicit-session list is isolated", func(t *testing.T) {
		assertEmptyWorkerSessionList(t, ctx, process, env, factoryDir, baseURL, emptyFactorySessionID)
	})

	successWorkID := submitWork(t, ctx, process, env, factoryDir, baseURL, successFactorySessionID, "worker-session-cli-success")
	waitForWorkerSession(t, ctx, process, env, factoryDir, baseURL, successFactorySessionID, successWorkID)
	streamWorkerSession(t, ctx, process, env, factoryDir, baseURL, successFactorySessionID, workerSessionsCodexSuccessID, "COMPLETED")
	assertSuccessfulWorkerSession(t, ctx, process, env, factoryDir, baseURL, successFactorySessionID, successWorkID)
	caseFixture.closeRoute(t, "worker-session-cli-success")

	caseFixture.registerRoutes(t, "worker-session-cli-failure")
	failureWorkID := submitWork(t, ctx, process, env, factoryDir, baseURL, failureFactorySessionID, "worker-session-cli-failure")
	waitForWorkerSession(t, ctx, process, env, factoryDir, baseURL, failureFactorySessionID, failureWorkID)
	streamWorkerSession(t, ctx, process, env, factoryDir, baseURL, failureFactorySessionID, workerSessionsCodexFailureID, "FAILED")
	assertFailedWorkerSession(t, ctx, process, env, factoryDir, baseURL, failureFactorySessionID, failureWorkID)
	caseFixture.closeRoute(t, "worker-session-cli-failure")

	caseFixture.registerRoutes(t, "worker-session-cli-recovery-success")
	recoveryFactorySessionID := caseFixture.openSession(t)
	recoveryWorkID := submitWork(t, ctx, process, env, factoryDir, baseURL, recoveryFactorySessionID, "worker-session-cli-recovery-success")
	waitForWorkerSession(t, ctx, process, env, factoryDir, baseURL, recoveryFactorySessionID, recoveryWorkID)
	streamWorkerSession(t, ctx, process, env, factoryDir, baseURL, recoveryFactorySessionID, "session_fixture_codex_recovery_success", "COMPLETED")
	t.Run("FT-R02 failure recovers into an isolated success", func(t *testing.T) {
		assertSuccessfulWorkerSessionWithProvider(t, ctx, process, env, factoryDir, baseURL, recoveryFactorySessionID, recoveryWorkID, "session_fixture_codex_recovery_success")
	})
	caseFixture.closeRoute(t, "worker-session-cli-recovery-success")
	assertFleetWorkerSessionList(t, ctx, process, env, factoryDir, baseURL, map[string]string{
		successWorkID:  successFactorySessionID,
		failureWorkID:  failureFactorySessionID,
		recoveryWorkID: recoveryFactorySessionID,
	}, map[string]string{
		successWorkID:  "worker-session-cli-success",
		failureWorkID:  "worker-session-cli-failure",
		recoveryWorkID: "worker-session-cli-recovery-success",
	}, map[string]string{
		successWorkID:  workerSessionsCodexSuccessID,
		failureWorkID:  workerSessionsCodexFailureID,
		recoveryWorkID: "session_fixture_codex_recovery_success",
	}, true)
	t.Run("FT-B05 repeated reads preserve history and identity", func(t *testing.T) {
		assertRepeatedWorkerSessionReads(t, ctx, process, env, factoryDir, baseURL, successFactorySessionID, workerSessionsCodexSuccessID, successWorkID, fixture)
	})
	assertMissingWorkerSessionOutcomes(t, ctx, process, env, factoryDir, baseURL, successFactorySessionID, fixture)
	assertMissingWorkerSessionInputs(t, ctx, process, env, factoryDir, baseURL)

	assertProviderCommandRoutesSince(t, fixture.runner, routeStart, map[string]struct{}{
		"worker-session-cli-success":          {},
		"worker-session-cli-failure":          {},
		"worker-session-cli-recovery-success": {},
	})
	recordPath := fixture.recordPath
	if info, err := os.Stat(recordPath); err != nil {
		t.Fatalf("recorded worker activity missing at %s: %v", recordPath, err)
	} else if info.IsDir() || info.Size() == 0 {
		t.Fatalf("recorded worker activity at %s = directory=%t size=%d, want non-empty file", recordPath, info.IsDir(), info.Size())
	}
	replayFixture := workerSessionReplayFixture{
		process: process, factoryDir: factoryDir, env: env, baseURL: baseURL,
		sessionID: successFactorySessionID, requestID: "worker-session-cli-success",
		providerSessionID: workerSessionsCodexSuccessID, runner: fixture.runner,
	}
	frames := replayWorkerSessionFrames(t, ctx, replayFixture)
	assertWSRWorkerSessionHistory(t, frames, successFactorySessionID, successWorkID, "COMPLETED")

	functionalevidence.Covers(t,
		"cli/you.worker-sessions.list",
		"cli/you.worker-sessions.read",
		"cli/you.worker-sessions.show",
		"cli/you.worker-sessions.stream",
	)
}

func assertWorkerSessionsCLIHelp(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir string) {
	t.Helper()
	helpInputs := executeCLI(t, ctx, process, env, factoryDir, "worker-sessions")
	for _, marker := range []string{
		"Usage:",
		"continue    Continue a Worker Session",
		"invoke      Invoke a direct Worker",
		"list        List fleet-wide or Work-scoped Worker Sessions",
		"read        Read a finished Worker Session",
		"show        Show one Worker Session",
		"stream      Stream one Worker Session",
	} {
		if !strings.Contains(helpInputs.Stdout(), marker) {
			t.Fatalf("worker-sessions help omitted %q:\n%s", marker, helpInputs.Stdout())
		}
	}
	explicitHelpInputs := executeCLI(t, ctx, process, env, factoryDir, "worker-sessions", "--help")
	if !strings.Contains(explicitHelpInputs.Stdout(), "Usage:") {
		t.Fatalf("worker-sessions --help omitted usage:\n%s", explicitHelpInputs.Stdout())
	}
	listHelpInputs := executeCLI(t, ctx, process, env, factoryDir, "worker-sessions", "list", "--help")
	for _, marker := range []string{"--work-id", "--state", "--limit", "fleet-wide", "provider-session"} {
		if !strings.Contains(strings.ToLower(listHelpInputs.Stdout()), strings.ToLower(marker)) {
			t.Fatalf("worker-sessions list help omitted %q:\n%s", marker, listHelpInputs.Stdout())
		}
	}
	completionInputs := executeCLI(t, ctx, process, env, factoryDir, "__complete", "worker-sessions", "")
	for _, marker := range []string{"continue", "invoke", "list", "read", "show", "stream"} {
		if !strings.Contains(completionInputs.Stdout(), marker) {
			t.Fatalf("worker-sessions completion omitted %q:\n%s", marker, completionInputs.Stdout())
		}
	}
}

func assertWorkerSessionsCLIUnknown(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir string) {
	t.Helper()
	unknownInputs, unknownErr := executeCLIExpectError(t, ctx, process, env, factoryDir, "worker-sessions", "--unknown")
	if unknownErr == nil {
		t.Fatal("worker-sessions unknown argument returned nil error")
	}
	if !strings.Contains(unknownErr.Error()+unknownInputs.Stderr(), "unknown command") {
		t.Fatalf("worker-sessions unknown argument omitted diagnostic: %v\nstderr:\n%s", unknownErr, unknownInputs.Stderr())
	}
}

// TestWorkerSessionsReplayOnlyRedirectsWellFormedNDJSON proves that the
// published you process can finish a finite replay through shell redirection
// without cancellation or diagnostics contaminating stdout.
func TestWorkerSessionsReplayOnlyRedirectsWellFormedNDJSON(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	fixture := newWorkerSessionReplayFixture(t, ctx, "worker-session-replay-only-redirect", "session_fixture_codex_replay_redirect")
	defer fixture.stop(t)
	workID := submitWork(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, "worker-session-replay-only-redirect")
	waitForWorkerSession(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, workID)
	streamWorkerSession(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, fixture.providerSessionID, "COMPLETED")

	contents, diagnostics := runBuiltWorkerSessionReplay(t, ctx, fixture)
	assertWorkerSessionReplayCapture(t, contents, diagnostics)
	assertProviderCommandRoutesSince(t, fixture.runner, fixture.routeStart, map[string]struct{}{fixture.requestID: {}})
	fixture.caseFixture.closeRoute(t, fixture.requestID)
}

type workerSessionReplayFixture struct {
	caseFixture       *workerSessionsCLICase
	process           support.Process
	factoryDir        string
	env               []string
	baseURL           string
	sessionID         string
	requestID         string
	providerSessionID string
	runner            *providerCommandRouteRunner
	routeStart        int
}

// TestWSRFT001OpeningRecordPrecedesProviderOutput exercises the customer
// Worker Session stream against the production root-built process and asserts
// the retained history order directly. The provider command runner replays the
// sanitized Codex fixture; no Mock Worker or timing sleep participates in the
// observation.
//
// WSR-FT-001: opening-first, provider-before-output, terminal-last.
// golden: tests/functional/internal/support/testdata/provider-sessions/codex/success/manifest.json
func TestWSRFT001OpeningRecordPrecedesProviderOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	fixture := newWorkerSessionReplayFixture(t, ctx, "wsr-ft-001", "session_fixture_codex_wsr_ft_001")
	defer fixture.stop(t)
	workID := submitWork(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, "wsr-ft-001")
	waitForWorkerSession(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, workID)
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, fixture.sessionID, 30*time.Second)
	assertScopedWorkerSessionList(t, listWorkerSessionsForFactorySession(t, fixture.baseURL, fixture.sessionID, workID), fixture.sessionID, fixture.providerSessionID, workID)
	frames := replayWorkerSessionFrames(t, ctx, fixture)
	assertWSRWorkerSessionHistory(t, frames, fixture.sessionID, workID, "COMPLETED")
	assertProviderCommandRoutesSince(t, fixture.runner, fixture.routeStart, map[string]struct{}{fixture.requestID: {}})
	fixture.caseFixture.closeRoute(t, fixture.requestID)
}

// TestWSRFT002LiveAndReplayCorrelationRemainStable compares the public live
// Worker Session observation with the replay-only stream. The opening's exact
// timestamp and identity must survive both projections, while the stream's
// provider-native records remain after the opening lifecycle record.
//
// WSR-FT-002: live/replay correlation and exact opening timestamp.
func TestWSRFT002LiveAndReplayCorrelationRemainStable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	fixture := newWorkerSessionReplayFixture(t, ctx, "wsr-ft-002", "session_fixture_codex_wsr_ft_002")
	defer fixture.stop(t)
	workID := submitWork(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, "wsr-ft-002")
	waitForWorkerSession(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, workID)
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, fixture.sessionID, 30*time.Second)
	live := listWorkerSessionsForFactorySession(t, fixture.baseURL, fixture.sessionID, workID)
	if len(live.Sessions) != 1 {
		t.Fatalf("live Worker Session observations = %#v, want exactly one", live)
	}
	assertScopedWorkerSessionList(t, live, fixture.sessionID, fixture.providerSessionID, workID)
	frames := replayWorkerSessionFrames(t, ctx, fixture)
	assertWSRLiveReplayCorrelation(t, live.Sessions[0], frames, fixture.sessionID, workID)
	assertProviderCommandRoutesSince(t, fixture.runner, fixture.routeStart, map[string]struct{}{fixture.requestID: {}})
	fixture.caseFixture.closeRoute(t, fixture.requestID)
}

func replayWorkerSessionFrames(
	t *testing.T,
	ctx context.Context,
	fixture workerSessionReplayFixture,
) []factoryapi.WorkerSessionEvent {
	t.Helper()
	inputs := executeCLI(t, ctx, fixture.process, fixture.env, fixture.factoryDir,
		"--server", fixture.baseURL, "worker-sessions", "stream",
		"--session", fixture.sessionID, "--provider", "codex", "--kind", "session_id", "--id", fixture.providerSessionID,
		"--replay-only", "--output", "json",
	)
	var frames []factoryapi.WorkerSessionEvent
	for index, line := range nonEmptyLines(inputs.Stdout()) {
		var frame factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("decode WSR replay frame %d: %v\nline:%s", index+1, err, line)
		}
		if frame.Event.Position == 0 || frame.WorkerSessionId == "" {
			continue
		}
		frames = append(frames, frame)
	}
	if len(frames) == 0 {
		t.Fatalf("replay stream contained no Worker Session records:\n%s", inputs.Stdout())
	}
	return frames
}

func assertWSRWorkerSessionHistory(
	t *testing.T,
	frames []factoryapi.WorkerSessionEvent,
	factorySessionID string,
	workID string,
	wantTerminal string,
) {
	t.Helper()
	if frames[0].Event.SourceType == "factory_event" {
		assertCanonicalWorkerSessionHistory(t, frames, workID, wantTerminal)
		return
	}
	if frames[0].Event.Position != 1 {
		t.Fatalf("first Worker Session position = %d, want 1", frames[0].Event.Position)
	}
	if frames[0].Event.SourceType != "worker_session_lifecycle" {
		t.Fatalf("first Worker Session source type = %q, want worker_session_lifecycle", frames[0].Event.SourceType)
	}
	if got := stringValue(frames[0].Event.Payload, "kind"); got != "SESSION" || stringValue(frames[0].Event.Payload, "phase") != "STARTED" {
		t.Fatalf("first Worker Session payload = %#v, want SESSION/STARTED", frames[0].Event.Payload)
	}
	providerOutputSeen := false
	providerBindingSeen := false
	providerBindingIndex := -1
	firstProviderOutput := -1
	terminalSeen := false
	for index, frame := range frames {
		if frame.WorkerSessionId != frames[0].WorkerSessionId || !containsString(frame.WorkIds, workID) {
			t.Fatalf("frame[%d] correlation = %#v, want worker %s and Work %s", index, frame, frames[0].WorkerSessionId, workID)
		}
		if index > 0 && frame.Event.Position <= frames[index-1].Event.Position {
			t.Fatalf("Worker Session positions are not increasing: frame[%d]=%d previous=%d", index, frame.Event.Position, frames[index-1].Event.Position)
		}
		if frame.Event.SourceType == "worker_session_lifecycle" &&
			stringValue(frame.Event.Payload, "phase") == "UPDATED" &&
			providerValue(frame.Event.Payload) == "codex" {
			providerBindingSeen = true
			if providerBindingIndex == -1 {
				providerBindingIndex = index
			}
		}
		if frame.Event.SourceType != "worker_session_lifecycle" {
			providerOutputSeen = true
			if firstProviderOutput == -1 {
				firstProviderOutput = index
			}
		}
		if frame.Event.SourceType == "worker_session_lifecycle" &&
			(stringValue(frame.Event.Payload, "phase") == "COMPLETED" ||
				stringValue(frame.Event.Payload, "phase") == "FAILED" ||
				stringValue(frame.Event.Payload, "phase") == "CANCELED") {
			if terminalSeen {
				t.Fatalf("Worker Session history has multiple terminal lifecycle records: %#v", frames)
			}
			terminalSeen = true
			if stringValue(frame.Event.Payload, "status") != wantTerminal {
				t.Fatalf("terminal Worker Session status = %q, want %q; payload=%#v", stringValue(frame.Event.Payload, "status"), wantTerminal, frame.Event.Payload)
			}
			if index != len(frames)-1 {
				t.Fatalf("terminal lifecycle record at frame %d, want final frame %d", index, len(frames)-1)
			}
		}
	}
	if !providerOutputSeen {
		t.Fatalf("Worker Session history has no provider-authored records: %#v", frames)
	}
	if !providerBindingSeen || firstProviderOutput == -1 || providerBindingIndex >= firstProviderOutput {
		t.Fatalf("Worker Session history did not bind codex before provider output: %#v", frames)
	}
	if !terminalSeen {
		t.Fatalf("Worker Session history has no terminal lifecycle record: %#v", frames)
	}
}

func assertWSRLiveReplayCorrelation(
	t *testing.T,
	live factoryapi.WorkerSessionObservation,
	frames []factoryapi.WorkerSessionEvent,
	factorySessionID string,
	workID string,
) {
	t.Helper()
	assertWSRWorkerSessionHistory(t, frames, factorySessionID, workID, "COMPLETED")
	if frames[0].Event.SourceType == "factory_event" {
		if live.WorkerSessionId != frames[0].WorkerSessionId || live.StartedAt == nil {
			t.Fatalf("live Worker Session = %#v, replay opening = %#v", live, frames[0])
		}
		// Canonical Factory-event payloads preserve dispatch correlation and
		// ordering; lifecycle startedAt is intentionally a projection field and
		// is not duplicated into every source event payload.
		return
	}
	if live.WorkerSessionId != frames[0].WorkerSessionId || live.State != factoryapi.WorkerSessionObservationStateCompleted {
		t.Fatalf("live Worker Session = %#v, replay opening = %#v", live, frames[0])
	}
	if live.StartedAt == nil {
		t.Fatal("live Worker Session omitted startedAt")
	}
	startedAt := stringValue(frames[0].Event.Payload, "startedAt")
	if startedAt == "" {
		t.Fatalf("replay opening omitted startedAt: %#v", frames[0].Event.Payload)
	}
	parsed, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		t.Fatalf("parse replay opening startedAt %q: %v", startedAt, err)
	}
	if !parsed.Equal(*live.StartedAt) {
		t.Fatalf("live startedAt = %s, replay startedAt = %s; live=%#v opening=%#v", live.StartedAt.Format(time.RFC3339Nano), parsed.Format(time.RFC3339Nano), live, frames[0].Event.Payload)
	}
	if live.AttemptId != stringValue(frames[0].Event.Payload, "attemptId") {
		t.Fatalf("live attemptId = %q, replay attemptId = %q", live.AttemptId, stringValue(frames[0].Event.Payload, "attemptId"))
	}
}

func listWorkerSessionsForFactorySession(t *testing.T, baseURL, factorySessionID, workID string) factoryapi.ListWorkerSessionsResponse {
	t.Helper()
	if strings.TrimSpace(factorySessionID) == "" || strings.TrimSpace(workID) == "" {
		t.Fatal("Factory Session and Work identities are required for a scoped Worker Session list")
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(factorySessionID) + "/worker-sessions?workId=" + url.QueryEscape(workID)
	return support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, endpoint)
}

func assertEmptyWorkerSessionList(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	factoryDir, baseURL, factorySessionID string,
) {
	t.Helper()
	inputs := executeCLI(t, ctx, process, env, factoryDir,
		"--server", baseURL, "worker-sessions", "list", "--session", factorySessionID, "--output", "json",
	)
	var listed workerSessionListJSON
	decodeCLIJSON(t, inputs, &listed)
	if listed.Sessions == nil {
		t.Fatalf("empty Factory Session Worker Session list is nil: %#v", listed)
	}
	if len(listed.Sessions) != 0 {
		t.Fatalf("empty Factory Session Worker Session list = %#v, want non-nil empty sessions", listed)
	}
}

func assertScopedWorkerSessionList(t *testing.T, listed factoryapi.ListWorkerSessionsResponse, factorySessionID, providerSessionID, workID string) {
	t.Helper()
	if len(listed.Sessions) != 1 {
		t.Fatalf("scoped Worker Session list = %#v, want exactly one observation", listed)
	}
	session := listed.Sessions[0]
	if session.FactorySessionId == nil || *session.FactorySessionId != factorySessionID {
		t.Fatalf("scoped Worker Session Factory Session = %#v, want %s", session.FactorySessionId, factorySessionID)
	}
	if session.WorkId == nil || *session.WorkId != workID {
		t.Fatalf("scoped Worker Session Work = %#v, want %s", session.WorkId, workID)
	}
	if session.ProviderSession == nil || session.ProviderSession.Id != providerSessionID {
		t.Fatalf("scoped Worker Session provider identity = %#v, want %s", session.ProviderSession, providerSessionID)
	}
}

func stringValue(payload map[string]interface{}, key string) string {
	if value, ok := payload[key].(string); ok {
		return value
	}
	if nested, ok := payload["payload"].(map[string]interface{}); ok {
		value, _ := nested[key].(string)
		return value
	}
	return ""
}

func providerValue(payload map[string]interface{}) string {
	provenance, _ := payload["provenance"].(map[string]interface{})
	provider, _ := provenance["provider"].(string)
	return provider
}

func newWorkerSessionReplayFixture(t *testing.T, ctx context.Context, requestID, providerSessionID string) workerSessionReplayFixture {
	t.Helper()
	caseFixture := newWorkerSessionsCLICase(t)
	shared := caseFixture.fixture
	caseFixture.registerRoutes(t, requestID)
	routeStart := shared.runner.CallCount()
	sessionID := caseFixture.openSession(t)
	return workerSessionReplayFixture{
		caseFixture: caseFixture,
		process:     shared.process,
		factoryDir:  caseFixture.factoryDir,
		env:         functionalEnvironment(shared.homeDir), baseURL: shared.baseURL,
		sessionID: sessionID, requestID: requestID, providerSessionID: providerSessionID,
		runner: shared.runner, routeStart: routeStart,
	}
}

func (fixture workerSessionReplayFixture) stop(t *testing.T) {
	t.Helper()
	fixture.caseFixture.cleanup(t)
}

func runBuiltWorkerSessionReplay(t *testing.T, ctx context.Context, fixture workerSessionReplayFixture) ([]byte, string) {
	t.Helper()
	capturePath := filepath.Join(t.TempDir(), "worker-session.jsonl")
	capture, err := os.Create(capturePath)
	if err != nil {
		t.Fatalf("create replay capture: %v", err)
	}
	defer capture.Close()
	var diagnostics bytes.Buffer
	command := exec.CommandContext(ctx, buildWorkerSessionsCLIBinary(t),
		"--verbose", "--server", fixture.baseURL, "worker-sessions", "stream",
		"--session", fixture.sessionID, "--provider", "codex", "--kind", "session_id", "--id", fixture.providerSessionID,
		"--replay-only", "--output", "json",
	)
	command.Dir = fixture.factoryDir
	command.Env = fixture.env
	command.Stdout = capture
	command.Stderr = &diagnostics
	if err := command.Run(); err != nil {
		t.Fatalf("built you replay-only stream: %v\nstderr:\n%s", err, diagnostics.String())
	}
	if err := capture.Close(); err != nil {
		t.Fatalf("close replay capture: %v", err)
	}
	contents, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("read replay capture: %v", err)
	}
	return contents, diagnostics.String()
}

func assertWorkerSessionReplayCapture(t *testing.T, contents []byte, diagnostics string) {
	t.Helper()
	lines := nonEmptyLines(string(contents))
	if len(lines) == 0 {
		t.Fatalf("replay capture is empty, want event records and a complete summary")
	}
	var previousPosition uint64
	eventsEmitted := 0
	var summary *workerSessionReplaySummaryJSON
	for index, line := range lines {
		var frame streamFrameJSON
		if err := json.Unmarshal([]byte(line), &frame); err == nil && frame.Event != nil {
			if frame.Delivery == "" {
				t.Fatalf("replay line %d = %#v, want an event delivery", index+1, frame)
			}
			if frame.Event.Position <= previousPosition {
				t.Fatalf("replay event positions are not canonical: previous=%d current=%d", previousPosition, frame.Event.Position)
			}
			previousPosition = frame.Event.Position
			eventsEmitted++
			if frame.ReplaySummary != nil {
				summary = frame.ReplaySummary
			}
			continue
		}
		var standalone workerSessionReplaySummaryJSON
		if err := json.Unmarshal([]byte(line), &standalone); err != nil {
			t.Fatalf("decode replay line %d: %v\nline:%s", index+1, err, line)
		}
		summary = &standalone
	}
	if eventsEmitted == 0 || summary == nil {
		t.Fatalf("replay capture omitted event records or summary: events=%d summary=%#v\n%s", eventsEmitted, summary, contents)
	}
	if summary.Kind != "replay-summary" || !summary.Complete ||
		(summary.Reason != "session-completed" && summary.Reason != "recording-complete") {
		t.Fatalf("replay summary = %#v, want complete terminal summary", *summary)
	}
	if summary.EventsEmitted != int64(eventsEmitted) {
		t.Fatalf("replay summary eventsEmitted = %d, want %d event records", summary.EventsEmitted, eventsEmitted)
	}
	if strings.Contains(string(contents), "worker sessions stream request") || strings.Contains(string(contents), "worker sessions stream response") {
		t.Fatalf("verbose diagnostics contaminated redirected stdout:\n%s", contents)
	}
	if strings.TrimSpace(diagnostics) == "" {
		t.Fatal("--verbose produced no stderr diagnostics to verify stream diagnostics stay off stdout")
	}
}

type workerSessionJSON struct {
	AttemptID        string               `json:"attemptId"`
	DurationMillis   *int64               `json:"durationMillis"`
	DurationBasis    string               `json:"durationBasis"`
	Failure          json.RawMessage      `json:"failure"`
	FactorySessionID *string              `json:"factorySessionId"`
	ProviderSession  *providerSessionJSON `json:"providerSession"`
	StartedAt        *time.Time           `json:"startedAt"`
	State            string               `json:"state"`
	TokenUsage       *tokenUsageJSON      `json:"tokenUsage"`
	Transcript       string               `json:"transcript"`
	WorkID           *string              `json:"workId"`
	WorkIDs          []string             `json:"workIds"`
	WorkName         *string              `json:"workName"`
	WorkerSessionID  string               `json:"workerSessionId"`
}

type providerSessionJSON struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
}

type tokenUsageJSON struct {
	InputTokens  *int `json:"inputTokens"`
	OutputTokens *int `json:"outputTokens"`
	TotalTokens  *int `json:"totalTokens"`
}

type workerSessionListJSON struct {
	Sessions []workerSessionJSON `json:"sessions"`
}

type transcriptJSON struct {
	Entries         []transcriptEntryJSON `json:"entries"`
	ProviderSession providerSessionJSON   `json:"providerSession"`
	State           string                `json:"state"`
	WorkIDs         []string              `json:"workIds"`
	WorkerSessionID string                `json:"workerSessionId"`
}

type transcriptEntryJSON struct {
	Text    string `json:"text"`
	Summary string `json:"summary"`
	Type    string `json:"type"`
}

type workerSessionsCLIPublicState struct {
	factorySessions []string
	workItems       []string
	workerSessions  []string
	providerRoutes  int
}

func captureWorkerSessionsCLIPublicState(
	t *testing.T,
	fixture *workerSessionsCLISharedFixture,
	factorySessionIDs ...string,
) workerSessionsCLIPublicState {
	t.Helper()
	state := workerSessionsCLIPublicState{providerRoutes: fixture.runner.CallCount()}
	factorySessions := support.GetJSON[factoryapi.ListFactorySessionsResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions",
	)
	for _, session := range factorySessions.Sessions {
		state.factorySessions = append(state.factorySessions, session.Id+"|"+session.FolderPath+"|"+session.FactoryDir)
	}
	workerSessions := support.GetJSON[factoryapi.ListWorkerSessionsResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/worker-sessions",
	)
	for _, session := range workerSessions.Sessions {
		state.workerSessions = append(state.workerSessions, workerSessionPublicIdentity(session))
	}
	for _, factorySessionID := range factorySessionIDs {
		workList := support.GetJSON[factoryapi.ListWorkResponse](
			t,
			strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+url.PathEscape(factorySessionID)+"/work",
		)
		for _, work := range workList.Results {
			state.workItems = append(state.workItems, workPublicIdentity(work))
		}
	}
	sort.Strings(state.factorySessions)
	sort.Strings(state.workItems)
	sort.Strings(state.workerSessions)
	return state
}

func assertWorkerSessionsCLIPublicStateUnchanged(
	t *testing.T,
	before, after workerSessionsCLIPublicState,
) {
	t.Helper()
	if strings.Join(before.factorySessions, "\n") != strings.Join(after.factorySessions, "\n") {
		t.Fatalf("Factory Session identities changed: before=%v after=%v", before.factorySessions, after.factorySessions)
	}
	if strings.Join(before.workItems, "\n") != strings.Join(after.workItems, "\n") {
		t.Fatalf("Work identities changed: before=%v after=%v", before.workItems, after.workItems)
	}
	if strings.Join(before.workerSessions, "\n") != strings.Join(after.workerSessions, "\n") {
		t.Fatalf("Worker Session identities changed: before=%v after=%v", before.workerSessions, after.workerSessions)
	}
	if before.providerRoutes != after.providerRoutes {
		t.Fatalf("provider command calls changed: before=%d after=%d", before.providerRoutes, after.providerRoutes)
	}
}

func workerSessionPublicIdentity(session factoryapi.WorkerSessionObservation) string {
	providerID := ""
	if session.ProviderSession != nil {
		providerID = session.ProviderSession.Provider + ":" + session.ProviderSession.Kind + ":" + session.ProviderSession.Id
	}
	factorySessionID := ""
	if session.FactorySessionId != nil {
		factorySessionID = *session.FactorySessionId
	}
	workID := ""
	if session.WorkId != nil {
		workID = *session.WorkId
	}
	return session.WorkerSessionId + "|" + factorySessionID + "|" + workID + "|" + providerID
}

func workPublicIdentity(work factoryapi.Work) string {
	workID := ""
	if work.WorkId != nil {
		workID = *work.WorkId
	}
	requestID := ""
	if work.RequestId != nil {
		requestID = *work.RequestId
	}
	return workID + "|" + requestID + "|" + work.Name
}

func submitWork(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, sessionID, name string) string {
	t.Helper()
	request := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":"task","payload":{"title":%q}}]}`,
		name, name, name,
	)
	inputs := executeCLI(t, ctx, process, env, factoryDir, "--server", baseURL, "--json", "submit", "batch", "--session", sessionID, request)
	var response struct {
		WorkCount int `json:"workCount"`
		Works     []struct {
			WorkID string `json:"workId"`
		} `json:"works"`
	}
	decodeCLIJSON(t, inputs, &response)
	if response.WorkCount != 1 || len(response.Works) != 1 || strings.TrimSpace(response.Works[0].WorkID) == "" {
		t.Fatalf("submit batch response missing one accepted Work: %#v\noutput:\n%s", response, inputs.Stdout())
	}
	return response.Works[0].WorkID
}

func waitForWorkerSession(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, sessionID, workID string) {
	t.Helper()
	session := waitForWorkerSessionState(t, ctx, process, env, factoryDir, baseURL, sessionID, workID, "")
	if session.FactorySessionID == nil || *session.FactorySessionID != sessionID {
		t.Fatalf("Worker Session Factory Session = %#v, want %s", session.FactorySessionID, sessionID)
	}
}

func waitForWorkerSessionState(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, sessionID, workID, expectedState string) workerSessionJSON {
	t.Helper()

	// Work admission, opening, and terminal projection are separate asynchronous
	// runtime steps. Synchronize through the customer-facing list projection so
	// each assertion observes the requested lifecycle state; a fixed sleep would
	// make this coverage slower and still race under CI.
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	var lastOutput string
	for {
		inputs := support.FakeInputs(ctx, []string{
			"you", "--server", baseURL, "worker-sessions", "list", "--session", sessionID, "--work-id", workID, "--output", "json",
		})
		inputs.Input.Env = append([]string(nil), env...)
		inputs.Input.WorkingDirectory = factoryDir
		if err := process.Execute(inputs.Input); err == nil {
			var listed workerSessionListJSON
			if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &listed); decodeErr == nil && len(listed.Sessions) > 0 {
				if len(listed.Sessions) != 1 {
					t.Fatalf("Worker Session count for Work %s = %d, want 1: %#v", workID, len(listed.Sessions), listed)
				}
				for _, session := range listed.Sessions {
					if expectedState == "" || session.State == expectedState {
						return session
					}
				}
			}
			lastOutput = inputs.Stdout()
		} else {
			lastErr = err
			lastOutput = inputs.Stdout()
		}

		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for Worker Session for Work %s to reach state %q: err=%v stdout=%s", workID, expectedState, lastErr, lastOutput)
		case <-ctx.Done():
			t.Fatalf("waiting for Worker Session for Work %s to reach state %q canceled: %v", workID, expectedState, ctx.Err())
		}
	}
}

func assertSuccessfulWorkerSession(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, sessionID, workID string) {
	t.Helper()
	assertSuccessfulWorkerSessionWithProvider(t, ctx, process, env, factoryDir, baseURL, sessionID, workID, workerSessionsCodexSuccessID)
}

func assertSuccessfulWorkerSessionWithProvider(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, sessionID, workID, providerID string) {
	t.Helper()
	session := waitForWorkerSessionState(t, ctx, process, env, factoryDir, baseURL, sessionID, workID, "COMPLETED")
	assertWorkerSessionIdentity(t, session, sessionID, providerID, workID)
	if session.State != "COMPLETED" || session.AttemptID == "" || session.DurationMillis == nil || *session.DurationMillis < 0 {
		t.Fatalf("successful session lifecycle projection = %#v", session)
	}
	if session.DurationBasis != "RECORDED_TIMESTAMPS" {
		t.Fatalf("successful duration basis = %q, want RECORDED_TIMESTAMPS", session.DurationBasis)
	}
	if session.TokenUsage == nil || session.TokenUsage.InputTokens == nil || *session.TokenUsage.InputTokens != 8 ||
		session.TokenUsage.OutputTokens == nil || *session.TokenUsage.OutputTokens != 12 ||
		session.TokenUsage.TotalTokens == nil || *session.TokenUsage.TotalTokens != 20 {
		t.Fatalf("successful token usage = %#v, want 8 input, 12 output, 20 total", session.TokenUsage)
	}

	showInputs := executeCLI(t, ctx, process, env, factoryDir,
		"--server", baseURL, "worker-sessions", "show", "--session", sessionID, "--provider", "codex", "--kind", "session_id", "--id", providerID, "--output", "json")
	var shown workerSessionJSON
	decodeCLIJSON(t, showInputs, &shown)
	assertWorkerSessionIdentity(t, shown, sessionID, providerID, workID)
	if shown.DurationMillis == nil || shown.TokenUsage == nil || shown.TokenUsage.TotalTokens == nil || *shown.TokenUsage.TotalTokens != 20 {
		t.Fatalf("successful show omitted duration or token usage: %#v", shown)
	}

	readInputs := executeCLI(t, ctx, process, env, factoryDir,
		"--server", baseURL, "worker-sessions", "read", "--session", sessionID, "--provider", "codex", "--kind", "session_id", "--id", providerID, "--output", "json")
	var transcript transcriptJSON
	decodeCLIJSON(t, readInputs, &transcript)
	if transcript.ProviderSession.ID != providerID || !containsString(transcript.WorkIDs, workID) || len(transcript.Entries) == 0 {
		t.Fatalf("successful transcript correlation = %#v", transcript)
	}
	transcriptText, _ := json.Marshal(transcript.Entries)
	if !strings.Contains(string(transcriptText), "Codex fixture answer COMPLETE") {
		t.Fatalf("successful transcript omitted fixture answer:\n%s", transcriptText)
	}
}

func assertRepeatedWorkerSessionReads(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	factoryDir, baseURL, sessionID, providerID, workID string,
	fixture *workerSessionsCLISharedFixture,
) {
	t.Helper()
	before := captureWorkerSessionsCLIPublicState(t, fixture, sessionID)
	showArgs := []string{
		"--server", baseURL, "worker-sessions", "show", "--session", sessionID,
		"--provider", "codex", "--kind", "session_id", "--id", providerID, "--output", "json",
	}
	firstShow := executeCLI(t, ctx, process, env, factoryDir, showArgs...)
	secondShow := executeCLI(t, ctx, process, env, factoryDir, showArgs...)
	if strings.TrimSpace(firstShow.Stdout()) != strings.TrimSpace(secondShow.Stdout()) {
		t.Fatalf("repeated show output changed:\nfirst:\n%s\nsecond:\n%s", firstShow.Stdout(), secondShow.Stdout())
	}

	readArgs := []string{
		"--server", baseURL, "worker-sessions", "read", "--session", sessionID,
		"--provider", "codex", "--kind", "session_id", "--id", providerID, "--output", "json",
	}
	firstRead := executeCLI(t, ctx, process, env, factoryDir, readArgs...)
	secondRead := executeCLI(t, ctx, process, env, factoryDir, readArgs...)
	if strings.TrimSpace(firstRead.Stdout()) != strings.TrimSpace(secondRead.Stdout()) {
		t.Fatalf("repeated read output changed:\nfirst:\n%s\nsecond:\n%s", firstRead.Stdout(), secondRead.Stdout())
	}
	var transcript transcriptJSON
	decodeCLIJSON(t, firstRead, &transcript)
	if transcript.ProviderSession.ID != providerID || !containsString(transcript.WorkIDs, workID) || len(transcript.Entries) == 0 {
		t.Fatalf("repeated read transcript identity = %#v, want provider %s and Work %s", transcript, providerID, workID)
	}

	replayArgs := []string{
		"--verbose", "--server", baseURL, "worker-sessions", "stream", "--session", sessionID,
		"--provider", "codex", "--kind", "session_id", "--id", providerID, "--replay-only", "--output", "json",
	}
	firstReplay := executeCLI(t, ctx, process, env, factoryDir, replayArgs...)
	secondReplay := executeCLI(t, ctx, process, env, factoryDir, replayArgs...)
	assertWorkerSessionReplayCapture(t, []byte(firstReplay.Stdout()), firstReplay.Stderr())
	assertWorkerSessionReplayCapture(t, []byte(secondReplay.Stdout()), secondReplay.Stderr())
	if strings.TrimSpace(firstReplay.Stdout()) != strings.TrimSpace(secondReplay.Stdout()) {
		t.Fatalf("repeated replay output changed:\nfirst:\n%s\nsecond:\n%s", firstReplay.Stdout(), secondReplay.Stdout())
	}
	after := captureWorkerSessionsCLIPublicState(t, fixture, sessionID)
	assertWorkerSessionsCLIPublicStateUnchanged(t, before, after)
}

func assertFailedWorkerSession(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, sessionID, workID string) {
	t.Helper()
	session := waitForWorkerSessionState(t, ctx, process, env, factoryDir, baseURL, sessionID, workID, "FAILED")
	assertWorkerSessionIdentity(t, session, sessionID, workerSessionsCodexFailureID, workID)
	if session.State != "FAILED" || session.AttemptID == "" || session.DurationMillis == nil || *session.DurationMillis < 0 {
		t.Fatalf("failed session lifecycle projection = %#v", session)
	}
	if !strings.Contains(strings.ToLower(string(session.Failure)), "auth") && !strings.Contains(strings.ToLower(string(session.Failure)), "401") {
		t.Fatalf("failed session omitted authentication diagnosis: %s", session.Failure)
	}

	showInputs := executeCLI(t, ctx, process, env, factoryDir,
		"--server", baseURL, "worker-sessions", "show", "--session", sessionID, "--provider", "codex", "--kind", "session_id", "--id", workerSessionsCodexFailureID, "--output", "json")
	var shown workerSessionJSON
	decodeCLIJSON(t, showInputs, &shown)
	assertWorkerSessionIdentity(t, shown, sessionID, workerSessionsCodexFailureID, workID)
	if shown.State != "FAILED" || (!strings.Contains(strings.ToLower(string(shown.Failure)), "auth") && !strings.Contains(strings.ToLower(string(shown.Failure)), "401")) {
		t.Fatalf("failed show omitted recorded failure cause: %#v", shown)
	}
}

func assertMissingWorkerSessionOutcomes(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, sessionID string, fixture *workerSessionsCLISharedFixture) {
	t.Helper()
	cases := []struct {
		name string
		args []string
		code string
	}{
		{
			name: "list missing Work",
			args: []string{"worker-sessions", "list", "--session", sessionID, "--work-id", "work-missing-from-cli", "--output", "json"},
			code: "WORK_NOT_FOUND",
		},
		{
			name: "show missing session",
			args: []string{"worker-sessions", "show", "--session", sessionID, "--provider", "codex", "--kind", "session_id", "--id", "provider-session-missing-from-cli", "--output", "json"},
			code: "WORKER_SESSION_NOT_FOUND",
		},
		{
			name: "read missing session",
			args: []string{"worker-sessions", "read", "--session", sessionID, "--provider", "codex", "--kind", "session_id", "--id", "provider-session-missing-from-cli", "--output", "json"},
			code: "WORKER_SESSION_NOT_FOUND",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			before := captureWorkerSessionsCLIPublicState(t, fixture, sessionID)
			inputs, err := executeCLIExpectError(t, ctx, process, env, factoryDir, append([]string{"--server", baseURL}, test.args...)...)
			if err == nil {
				t.Fatal("missing Worker Session operation returned nil error")
			}
			output := inputs.Stdout() + inputs.Stderr() + err.Error()
			if !strings.Contains(output, test.code) {
				t.Fatalf("missing Worker Session operation omitted %s: %s", test.code, output)
			}
			if strings.TrimSpace(inputs.Stdout()) != "" {
				t.Fatalf("missing Worker Session operation emitted stdout side effect: %s", inputs.Stdout())
			}
			after := captureWorkerSessionsCLIPublicState(t, fixture, sessionID)
			assertWorkerSessionsCLIPublicStateUnchanged(t, before, after)
		})
	}
}

func assertMissingWorkerSessionInputs(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL string) {
	t.Helper()
	cases := []struct {
		name string
		args []string
		code string
	}{
		{
			name: "show local provider validation",
			args: []string{"--server", baseURL, "worker-sessions", "show", "--output", "json"},
			code: "PROVIDER_REQUIRED",
		},
		{
			name: "show global kind validation",
			args: []string{"--json", "worker-sessions", "show", "--provider", "codex"},
			code: "SESSION_KIND_REQUIRED",
		},
		{
			name: "show local ID validation",
			args: []string{"--server", baseURL, "worker-sessions", "show", "--provider", "codex", "--kind", "session_id", "--output", "json"},
			code: "SESSION_ID_REQUIRED",
		},
		{
			name: "read local provider validation",
			args: []string{"--server", baseURL, "worker-sessions", "read", "--output", "json"},
			code: "PROVIDER_REQUIRED",
		},
		{
			name: "read global kind validation",
			args: []string{"--json", "worker-sessions", "read", "--provider", "codex"},
			code: "SESSION_KIND_REQUIRED",
		},
		{
			name: "read local ID validation",
			args: []string{"--server", baseURL, "worker-sessions", "read", "--provider", "codex", "--kind", "session_id", "--output", "json"},
			code: "SESSION_ID_REQUIRED",
		},
		{
			name: "stream local provider validation",
			args: []string{"--server", baseURL, "worker-sessions", "stream", "--output", "json"},
			code: "PROVIDER_REQUIRED",
		},
		{
			name: "stream global kind validation",
			args: []string{"--json", "worker-sessions", "stream", "--provider", "codex"},
			code: "SESSION_KIND_REQUIRED",
		},
		{
			name: "stream local ID validation",
			args: []string{"--server", baseURL, "worker-sessions", "stream", "--provider", "codex", "--kind", "session_id", "--output", "json"},
			code: "SESSION_ID_REQUIRED",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			inputs, err := executeCLIExpectError(t, ctx, process, env, factoryDir, test.args...)
			if err == nil {
				t.Fatal("missing required Worker Session input returned nil error")
			}
			var payload struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &payload); decodeErr != nil {
				t.Fatalf("decode required-input JSON: %v\nstdout:\n%s\nstderr:\n%s", decodeErr, inputs.Stdout(), inputs.Stderr())
			}
			if payload.Code != test.code || strings.TrimSpace(payload.Message) == "" {
				t.Fatalf("required-input payload = %#v, want code %s and message", payload, test.code)
			}
			if strings.Contains(inputs.Stdout(), "required flag(s)") || strings.Contains(inputs.Stderr(), "required flag(s)") {
				t.Fatalf("Cobra required-flag prose leaked into machine-readable failure:\nstdout:\n%s\nstderr:\n%s", inputs.Stdout(), inputs.Stderr())
			}
		})
	}
}

func assertWorkerSessionIdentity(t *testing.T, session workerSessionJSON, factorySessionID, providerID, workID string) {
	t.Helper()
	if session.WorkerSessionID == "" || session.ProviderSession == nil ||
		session.ProviderSession.Provider != "codex" || session.ProviderSession.Kind != "session_id" ||
		session.ProviderSession.ID != providerID || !containsString(session.WorkIDs, workID) ||
		session.FactorySessionID == nil || *session.FactorySessionID != factorySessionID {
		t.Fatalf("worker session identity = %#v, want Factory Session %s, provider %s, and Work %s", session, factorySessionID, providerID, workID)
	}
}

func streamWorkerSession(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, factorySessionID, providerID, terminalState string) {
	t.Helper()

	// The list projection can observe the opening before the stream's
	// provider-session lookup has installed its in-memory association. Retry the
	// same public stream operation while that lookup reports not-found; once the
	// stream opens, any other error remains an actionable test failure.
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var inputs *support.CapturedInputs
	var command *support.ProcessCommand
	for {
		inputs = support.FakeInputs(ctx, []string{
			"you", "--server", baseURL, "worker-sessions", "stream",
			"--session", factorySessionID, "--provider", "codex", "--kind", "session_id", "--id", providerID, "--output", "json",
		})
		inputs.Input.Env = append([]string(nil), env...)
		inputs.Input.WorkingDirectory = factoryDir
		command = support.StartProcessCommand(t, process, inputs.Input)
		select {
		case <-command.Done():
			err := command.Err()
			if err == nil {
				goto streamReady
			}
			if !strings.Contains(inputs.Stderr()+err.Error(), "WORKER_SESSION_NOT_FOUND") {
				t.Fatalf("worker-sessions stream %s: %v\nstdout:\n%s\nstderr:\n%s", providerID, err, inputs.Stdout(), inputs.Stderr())
			}
			command.AcceptError()
		case <-ctx.Done():
			command.Stop(t)
			t.Fatalf("worker-sessions stream %s timed out: %v", providerID, ctx.Err())
		}

		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for worker-sessions stream %s: stdout=%s stderr=%s", providerID, inputs.Stdout(), inputs.Stderr())
		case <-ctx.Done():
			t.Fatalf("waiting for worker-sessions stream %s canceled: %v", providerID, ctx.Err())
		}
	}

streamReady:
	if err := command.Err(); err != nil {
		t.Fatalf("worker-sessions stream %s: %v\nstdout:\n%s\nstderr:\n%s", providerID, err, inputs.Stdout(), inputs.Stderr())
	}

	scanner := bufio.NewScanner(strings.NewReader(inputs.Stdout()))
	var deliveries []string
	var frames []streamFrameJSON
	for scanner.Scan() {
		var frame streamFrameJSON
		if err := json.Unmarshal([]byte(scanner.Text()), &frame); err != nil {
			t.Fatalf("decode worker-sessions stream frame: %v\nframe:%s", err, scanner.Text())
		}
		deliveries = append(deliveries, frame.Delivery)
		frames = append(frames, frame)
		if frame.ProviderSession == nil || frame.ProviderSession.ID != providerID {
			t.Fatalf("stream frame provider session = %#v, want %s", frame.ProviderSession, providerID)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan worker-sessions stream output: %v", err)
	}
	if !containsString(deliveries, "RECORD") && !containsString(deliveries, "RECORD_REPLAY") {
		t.Fatalf("worker-sessions stream omitted activity frame: %v\noutput:\n%s", deliveries, inputs.Stdout())
	}
	if !containsString(deliveries, "TERMINAL") && !containsString(deliveries, "TERMINAL_REPLAY") {
		t.Fatalf("worker-sessions stream omitted terminal frame: %v\noutput:\n%s", deliveries, inputs.Stdout())
	}
	streamOutput := inputs.Stdout()
	if len(deliveries) > 0 && len(frames) > 0 && frames[0].Event != nil && frames[0].Event.SourceType == "factory_event" {
		assertCanonicalWorkerSessionStream(t, frames, providerID, terminalState)
		return
	}
	if !strings.Contains(streamOutput, `"phase":"STARTED"`) || !strings.Contains(streamOutput, `"status":"`+terminalState+`"`) {
		t.Fatalf("worker-sessions stream omitted active or terminal state %s: %s", terminalState, streamOutput)
	}
}

func executeCLI(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir string, args ...string) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(ctx, append([]string{"you"}, args...))
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("you %s: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs
}

func executeCLIExpectError(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir string, args ...string) (*support.CapturedInputs, error) {
	t.Helper()
	inputs := support.FakeInputs(ctx, append([]string{"you"}, args...))
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = factoryDir
	return inputs, process.Execute(inputs.Input)
}

func decodeCLIJSON(t *testing.T, inputs *support.CapturedInputs, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), target); err != nil {
		t.Fatalf("decode CLI JSON: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
}

func readProviderFixture(t *testing.T, provider, caseName, fileName string) []byte {
	t.Helper()
	path := filepath.Join(testutil.MustRepoRoot(t), filepath.FromSlash(support.ProviderSessionFixturePath(provider, caseName, fileName)))
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read provider fixture %s: %v", path, err)
	}
	return contents
}

func writeCodexRollout(t *testing.T, homeDir, sessionID string, contents []byte) {
	t.Helper()
	directory := filepath.Join(homeDir, ".codex", "sessions", "2026", "07", "27")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create Codex session directory: %v", err)
	}
	path := filepath.Join(directory, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write Codex rollout fixture: %v", err)
	}
}

func functionalEnvironment(homeDir string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return env
}

func buildWorkerSessionsCLIBinary(t *testing.T) string {
	t.Helper()
	return cachedWorkerSessionsCLIBinary(t)
}

func nonEmptyLines(contents string) []string {
	lines := make([]string, 0)
	for _, line := range strings.Split(contents, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

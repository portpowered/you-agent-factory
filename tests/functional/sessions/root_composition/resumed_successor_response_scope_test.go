//go:build factoryartifact

package root_composition_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	resumedResponseScopeRecordingEnv = "INFINITE_YOU_RESUME_RESPONSE_SCOPE_RECORDING"
	resumedResponseScopeArchiveEnv   = "INFINITE_YOU_RESUME_RESPONSE_SCOPE_FACTORY_ARCHIVE"

	resumedResponseScopeRecordingSHA256 = "A4CE2FD1F587573224DB5283F797B12FA549315CEBB1E152AA3B6CAC60873EE9"
	resumedResponseScopeRecordingBytes  = int64(23606575)
	resumedResponseScopeArchiveSHA256   = "CAF067534E9FCD1F536298F3273F0049FC925BE251D44C5AB9179082BBF968AD"
	resumedResponseScopeArchiveBytes    = int64(145960)

	resumedResponseScopeReservedWorkID   = "work-thoughts-213"
	resumedResponseScopeWorkType         = "thoughts"
	resumedResponseScopeSecretMarker     = "story003-untrusted-payload-secret"
	resumedResponseScopeGeneratedName    = "generated successor thought"
	resumedResponseScopeGeneratedMarker  = "generated-success-payload"
	resumedResponseScopeControlledOutput = `{"decision":"ACCEPTED","feedback":"controlled replay result","output":"done"}`
	resumedResponseScopeSuccessorID      = "00000000-0000-4000-8000-000000000003"
	resumedResponseScopeRuntimeID        = "00000000-0000-4000-8000-000000000004"

	resumedResponseScopeObservationTimeout = 90 * time.Second
	resumedResponseScopeStreamTimeout      = 90 * time.Second
	// Race-instrumented root construction can spend several minutes loading the
	// operator-staged recording and packaged Factory before the injected API
	// starter is reached. This is a startup ceiling only; live observations use
	// the bounded event-driven timeouts above.
	resumedResponseScopeStartupTimeout = 5 * time.Minute
)

var resumedResponseScopeWorkIDPattern = regexp.MustCompile(
	`\bwork-([A-Za-z0-9][A-Za-z0-9_-]*)-([0-9]+)\b`,
)

// TestResumedSuccessorResponseScopeAndWorkAdmission exercises one copied
// legacy recording through the production root composition. It keeps the
// provider edge gated until the public HTTP and CLI conflict snapshots have
// been taken, then admits one generated Work and observes its live streams.
// The large artifacts are intentionally operator-supplied rather than checked
// into the repository; the declared artifact-backed gate fails closed when
// they are unavailable rather than treating an unexecuted proof as success.
func TestResumedSuccessorResponseScopeAndWorkAdmission(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	artifacts := stageResumedResponseScopeArtifacts(t)
	gate := make(chan struct{})
	providerRunner := newResumedResponseScopeProviderRunner(gate)
	responseEventIDs := &atomic.Uint64{}
	successorPath := filepath.Join(t.TempDir(), "successor.jsonl")
	journey := startResumedResponseScopeJourney(t, artifacts, providerRunner, responseEventIDs, successorPath)
	assertResumedReservedWorkConflicts(t, journey)
	generatedWorkID, providerCallsBeforeGenerated := admitResumedGeneratedWork(t, journey)
	dispatchID := observeResumedGeneratedWork(t, journey, generatedWorkID, providerCallsBeforeGenerated)
	finishResumedResponseScopeJourney(t, journey, dispatchID)
}

type resumedResponseScopeJourney struct {
	artifacts       resumedResponseScopeArtifacts
	server          *support.FunctionalAPIServer
	providerRunner  *resumedResponseScopeProviderRunner
	session         factoryapi.FactorySession
	sessionID       string
	before          resumedScopeSnapshot
	canonicalStream *support.FactoryEventStream
	responseStream  *support.FactoryResponseEventStream
	successorPath   string
}

func startResumedResponseScopeJourney(
	t *testing.T,
	artifacts resumedResponseScopeArtifacts,
	providerRunner *resumedResponseScopeProviderRunner,
	responseEventIDs *atomic.Uint64,
	successorPath string,
) *resumedResponseScopeJourney {
	t.Helper()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                artifacts.factoryDir,
		WorkingDirectory:          artifacts.extractedRoot,
		FactoryConfigPath:         filepath.Join(artifacts.factoryDir, "factory.json"),
		ServerReadyTimeout:        resumedResponseScopeStartupTimeout,
		WaitForServiceModeRuntime: true,
		Args: []string{
			"--resume", artifacts.recordingCopy,
			"--record", successorPath,
		},
		Edges: serviceedges.Edges{
			FactorySessionIDGenerator: func() string {
				return resumedResponseScopeSuccessorID
			},
			FactorySessionRuntimeInstanceIDGenerator: func() string {
				return resumedResponseScopeRuntimeID
			},
			FactorySessionResponseEventIDGenerator: func() string {
				return fmt.Sprintf("story003-response-event-%d", responseEventIDs.Add(1))
			},
			ProviderCommandRunner: providerRunner,
			ScriptCommandRunner:   support.NewStaticSuccessCommandRunner("blocked"),
		},
	})
	baseURL := server.URL()
	assertResumedResponseScopeListener(t, baseURL)
	if err := providerRunner.WaitForCall(t.Context()); err != nil {
		t.Fatalf("wait for resumed provider dispatch: %v", err)
	}
	t.Log("resumed provider dispatch observed")
	session := support.GetDefaultSession(t, baseURL)
	assertResumedSuccessorIdentity(t, session)
	assertResumedSuccessorIdentityStable(t, session, support.GetDefaultSession(t, baseURL))
	sessionID := session.Id
	initialEvents := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	assertResumedHistoricalPrefix(t, initialEvents, artifacts.ledger)
	t.Logf("resumed historical prefix observed: %d events", len(initialEvents))
	before, beforeEvents, beforeResponseEvents := captureOpenResumedScopeSnapshot(t, baseURL, sessionID)
	if len(beforeEvents) == 0 {
		t.Fatal("resumed canonical event snapshot is empty")
	}
	t.Logf("resumed open scope snapshot captured: canonical=%d response=%d", len(beforeEvents), len(beforeResponseEvents))
	canonicalStream, responseStream := openResumedFutureStreams(t, baseURL, sessionID, beforeEvents, beforeResponseEvents)
	return &resumedResponseScopeJourney{
		artifacts: artifacts, server: server, providerRunner: providerRunner,
		session: session, sessionID: sessionID, before: before,
		canonicalStream: canonicalStream, responseStream: responseStream,
		successorPath: successorPath,
	}
}

func openResumedFutureStreams(
	t testing.TB,
	baseURL, sessionID string,
	beforeEvents []factoryapi.FactoryEvent,
	beforeResponseEvents []factoryapi.FactoryResponseEvent,
) (*support.FactoryEventStream, *support.FactoryResponseEventStream) {
	t.Helper()
	if len(beforeEvents) < 2 {
		t.Fatalf("resumed canonical event snapshot has %d events, want at least two for a replay cursor", len(beforeEvents))
	}
	// A one-event replay prefix makes the legacy-compatible SSE handler flush
	// its headers before the generated Work is admitted. The cursor itself is
	// still the acknowledged canonical event immediately before the baseline
	// tail, so no event is lost from the live subscription.
	canonicalCursor := resumedResponseScopeCursor(beforeEvents[len(beforeEvents)-2])
	canonicalStream := support.OpenFactoryEventStreamAt(
		t,
		support.SessionEventsURLWithCursor(baseURL, sessionID, canonicalCursor),
	)
	responseStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURLWithAfterSequence(
			baseURL, sessionID, lastResumedResponseSequence(beforeResponseEvents),
		),
	)
	t.Log("future canonical and response streams open")
	return canonicalStream, responseStream
}

func assertResumedReservedWorkConflicts(t testing.TB, journey *resumedResponseScopeJourney) {
	t.Helper()
	reserved := resumedResponseScopeWork(
		resumedResponseScopeReservedWorkID,
		"reserved historical work",
		resumedResponseScopeSecretMarker,
	)
	assertReservedWorkReadable(t, journey.server.URL(), journey.sessionID, reserved)
	assertResumedHTTPReservedWorkConflict(t, journey, reserved)
	assertResumedCLIReservedWorkConflict(t, journey)
}

func assertResumedHTTPReservedWorkConflict(t testing.TB, journey *resumedResponseScopeJourney, reserved factoryapi.Work) {
	t.Helper()
	request := factoryapi.WorkRequest{
		RequestId: "story003-http-reserved-conflict",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:     &[]factoryapi.Work{reserved},
	}
	status, body := putResumedWorkRequest(t, journey.server.URL(), journey.sessionID, request)
	assertWorkRequestConflict(t, status, body, "HTTP")
	after := captureOpenResumedScopeSnapshotOnly(t, journey.server.URL(), journey.sessionID)
	assertResumedScopeSnapshotUnchanged(t, journey.before, after, "HTTP reserved Work conflict")
	t.Log("HTTP reserved Work conflict observed with unchanged snapshot")
}

func assertResumedCLIReservedWorkConflict(t testing.TB, journey *resumedResponseScopeJourney) {
	t.Helper()
	request := factoryapi.WorkRequest{
		RequestId: "story003-cli-reserved-conflict",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{resumedResponseScopeWork(
			resumedResponseScopeReservedWorkID,
			"reserved historical work from CLI",
			resumedResponseScopeSecretMarker,
		)},
	}
	stdout, stderr, err := executeResumedResponseScopeCLI(t, journey.server, []string{
		"you", "--server", journey.server.URL(), "--json", "submit", "batch",
		"--session", journey.sessionID, resumedInlineJSON(t, request),
	})
	if err == nil {
		t.Fatal("CLI reserved Work conflict succeeded")
	}
	diagnostic := err.Error() + "\n" + stderr
	for _, marker := range []string{"batch submission failed (409)", "code=CONFLICT", "family=CONFLICT"} {
		if !strings.Contains(diagnostic, marker) {
			t.Fatalf("CLI reserved Work conflict missing %q: %s", marker, diagnostic)
		}
	}
	if stdout != "" || strings.Contains(diagnostic, resumedResponseScopeSecretMarker) {
		t.Fatalf("CLI reserved Work conflict response leaked success or payload: stdout=%q diagnostic=%s", stdout, diagnostic)
	}
	after := captureOpenResumedScopeSnapshotOnly(t, journey.server.URL(), journey.sessionID)
	assertResumedScopeSnapshotUnchanged(t, journey.before, after, "CLI reserved Work conflict")
	t.Log("CLI reserved Work conflict observed with unchanged snapshot")
}

func admitResumedGeneratedWork(t testing.TB, journey *resumedResponseScopeJourney) (string, int) {
	t.Helper()
	workType := "validation"
	workID := resumedResponseScopeGeneratedWorkID(journey.artifacts.ledger, workType)
	providerCallsBefore := journey.providerRunner.CallCount()
	request := factoryapi.WorkRequest{
		RequestId: "story003-generated-work",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{resumedResponseScopeWorkOfType(
			workType, workID, resumedResponseScopeGeneratedName, resumedResponseScopeGeneratedMarker,
		)},
	}
	t.Log("submitting generated Work")
	status, body := putResumedWorkRequest(t, journey.server.URL(), journey.sessionID, request)
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		t.Fatalf("generated Work admission status = %d: %s", status, strings.TrimSpace(string(body)))
	}
	var submitted factoryapi.UpsertWorkRequestResponse
	if err := json.Unmarshal(body, &submitted); err != nil {
		t.Fatalf("decode generated Work admission: %v", err)
	}
	if len(submitted.Works) != 1 || submitted.Works[0].WorkId != workID {
		t.Fatalf("generated Work admission = %#v, want generated Work ID %q", submitted, workID)
	}
	if workID == resumedResponseScopeReservedWorkID {
		t.Fatalf("generated Work reused reserved historical ID %q", workID)
	}
	waitContext, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := journey.providerRunner.WaitForCallCount(waitContext, providerCallsBefore+1); err != nil {
		t.Fatalf("wait for generated Work provider dispatch: %v", err)
	}
	t.Logf("generated Work admitted and dispatched: %s workDirs=%q", workID, journey.providerRunner.WorkDirs())
	return workID, providerCallsBefore
}

func observeResumedGeneratedWork(t testing.TB, journey *resumedResponseScopeJourney, workID string, providerCallsBefore int) string {
	t.Helper()
	journey.providerRunner.Release()
	waitContext, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := journey.providerRunner.WaitForReturnCount(waitContext, providerCallsBefore+1); err != nil {
		t.Fatalf("wait for generated ProviderCommandRunner return: %v", err)
	}
	futureEvents := readResumedFutureEventsUntilWorkTerminal(t, journey.canonicalStream, workID)
	dispatchID := assertGeneratedWorkCanonicalProgress(t, futureEvents, workID)
	workerSessionID := resumedWorkerSessionIDForDispatch(t, futureEvents, dispatchID)
	assertResumedWorkerSessionIdentity(t, journey.server, journey.sessionID, workID, journey.session.Runtime.StreamIdentity.FactorySessionID, workerSessionID)
	responseEvents := readResumedResponseEventsUntilDispatchTerminal(t, journey.responseStream, dispatchID)
	assertGeneratedWorkResponseProgress(t, responseEvents, dispatchID, journey.sessionID)
	generatedWork := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(
		journey.server.URL(), journey.sessionID, "/work/"+url.PathEscape(workID),
	))
	assertGeneratedWorkTerminal(t, generatedWork, workID)
	assertGeneratedSuffixesAboveHistory(t, journey.server.URL(), journey.sessionID, journey.artifacts.ledger)
	assertResumedResponseScopeNoWarnings(t, futureEvents, responseEvents)
	journey.providerRunner.AssertSafeRequests(t)
	t.Logf("generated Work terminal and response lifecycle observed: canonical=%d response=%d", len(futureEvents), len(responseEvents))
	return dispatchID
}

func finishResumedResponseScopeJourney(t *testing.T, journey *resumedResponseScopeJourney, dispatchID string) {
	t.Helper()
	status, body := postResumedLifecycleControl(t, journey.server.URL(), journey.sessionID, "terminate")
	assertResumedTerminationAccepted(t, status, body)
	support.WaitForSessionStopped(t, journey.server.URL(), journey.sessionID, resumedResponseScopeObservationTimeout)
	assertTerminalResumedSession(t, support.GetDefaultSession(t, journey.server.URL()))
	journey.responseStream.WaitClosed(resumedResponseScopeStreamTimeout)
	journey.responseStream.Close()
	terminalResponseEvents := readClosedResumedResponseEvents(t, journey.server.URL(), journey.sessionID)
	assertResumedResponseEventOrder(t, terminalResponseEvents, journey.sessionID, dispatchID)
	terminalCanonicalEvents := support.GetFactoryEventsForSessionAt(t, journey.server.URL(), journey.sessionID)
	assertResumedSuccessorLifecycle(t, terminalCanonicalEvents, journey.session.Runtime.StreamIdentity.FactorySessionID)
	t.Log("resumed successor terminal lifecycle observed")
	assertResumedTerminalLifecycleControls(t, journey, terminalResponseEvents)
	journey.server.Stop(t)
	assertRecordedSuccessorExists(t, journey.successorPath)
}

func assertResumedTerminationAccepted(t testing.TB, status int, body []byte) {
	t.Helper()
	if status == http.StatusConflict {
		var response factoryapi.FactorySessionLifecycleControlResponse
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatalf("decode automatic terminal lifecycle response: %v", err)
		}
		if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
			t.Fatalf("automatic terminal lifecycle conflict = %#v, want TERMINAL_SESSION", response)
		}
		return
	}
	if status != http.StatusOK && status != http.StatusAccepted {
		t.Fatalf("terminate successor Factory Session status = %d: %s", status, strings.TrimSpace(string(body)))
	}
}

func assertResumedTerminalLifecycleControls(
	t testing.TB,
	journey *resumedResponseScopeJourney,
	terminalResponseEvents []factoryapi.FactoryResponseEvent,
) {
	t.Helper()
	terminalSnapshot := captureClosedResumedScopeSnapshot(t, journey.server.URL(), journey.sessionID, terminalResponseEvents)
	httpStatus, httpBody := postResumedLifecycleControl(t, journey.server.URL(), journey.sessionID, "cancel")
	assertTerminalLifecycleConflict(t, httpStatus, httpBody, "HTTP")
	assertResumedScopeSnapshotUnchanged(t, terminalSnapshot, captureClosedResumedScopeSnapshot(t, journey.server.URL(), journey.sessionID, nil), "HTTP terminal lifecycle conflict")
	stdout, stderr, err := executeResumedResponseScopeCLI(t, journey.server, []string{
		"you", "--server", journey.server.URL(), "--json", "session", "cancel", journey.sessionID,
	})
	if err == nil {
		t.Fatal("CLI terminal lifecycle mutation succeeded")
	}
	if !strings.Contains(err.Error(), string(factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession)) || strings.Contains(err.Error()+"\n"+stderr, resumedResponseScopeSecretMarker) {
		t.Fatalf("CLI terminal lifecycle response = err:%v stderr:%q, want typed terminal conflict without payload", err, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("CLI terminal lifecycle conflict omitted typed response")
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatalf("decode CLI terminal lifecycle conflict: %v; stdout=%q", err, stdout)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("CLI terminal lifecycle response = %#v, want TERMINAL_SESSION", response)
	}
	assertResumedScopeSnapshotUnchanged(t, terminalSnapshot, captureClosedResumedScopeSnapshot(t, journey.server.URL(), journey.sessionID, nil), "CLI terminal lifecycle conflict")
}

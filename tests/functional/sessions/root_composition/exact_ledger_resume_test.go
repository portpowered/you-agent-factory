package root_composition_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testpath"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	exactLedgerSnapshotEnv       = "INFINITE_YOU_EXACT_LEDGER_SNAPSHOT"
	exactLedgerSnapshotDefault   = ".cache/exact-ledger/live-final-sol-snapshot-20260911T0618Z.jsonl"
	exactLedgerSnapshotSHA256    = "21D00963D5645A9799B90A22CB91046639A2AFEB4160125F29E436A0ECB64DC5"
	exactLedgerSnapshotBytes     = int64(20304421)
	exactLedgerParentWorkID      = "batch-operator-localai-delegation-restart-20260907-localai"
	exactLedgerHistoricalCycleID = "batch-localai-project-cycle-044-contract-identity-hold-20260907-localai"
	exactLedgerCurrentCycleID    = "batch-localai-project-cycle-093-active-delivery-barrier-20260910-localai"
)

var exactLedgerCurrentDependencyWorkIDs = []string{"work-task-19", "work-task-22", "work-task-15"}

type exactLedgerInitialState struct {
	sessionID             string
	lastEventSequence     int
	currentWorkerSessions []factoryapi.WorkerSessionObservation
}

// TestResumeExactLedgerCurrentProjectCycle exercises the immutable authority
// prefix through root.BuildProcess and the public Factory Session surfaces. The
// task-owned artifact is intentionally not committed; ordinary CI runs this
// cell as a documented skip unless the operator supplies the exact artifact.
func TestResumeExactLedgerCurrentProjectCycle(t *testing.T) {
	acquireRootCompositionFixtureSlot(t)
	snapshotPath, snapshotPayload := loadExactLedgerSnapshot(t)
	successorPath := filepath.Join(t.TempDir(), "exact-ledger-successor.jsonl")
	factoryDir := testpath.MustRepoPathFromCaller(t, 0, "factory")
	if _, err := os.Stat(filepath.Join(factoryDir, "factory.json")); err != nil {
		t.Fatalf("factory fixture: %v", err)
	}
	server := startExactLedgerServer(t, factoryDir, snapshotPath, snapshotPayload, successorPath)
	initial := prepareExactLedgerInitialState(t, server)
	completeExactLedgerCurrentCycle(t, server, initial)
	recoverExactLedgerParent(t, server, initial.sessionID)
	eventsAfter := exactLedgerEventsAfterSequence(server.GetFactoryEvents(t), initial.lastEventSequence)
	assertExactLedgerFeedback(t, eventsAfter)

	server.Stop(t)
	successorStat, err := os.Stat(successorPath)
	if err != nil {
		t.Fatalf("successor recording was not created: %v", err)
	}
	if successorStat.Size() == 0 {
		t.Fatal("successor recording is empty")
	}
	verifyExactLedgerIdentity(t, snapshotPath)
}

func loadExactLedgerSnapshot(t *testing.T) (string, []byte) {
	t.Helper()
	sourcePath := strings.TrimSpace(os.Getenv(exactLedgerSnapshotEnv))
	if sourcePath == "" {
		sourcePath = testpath.MustRepoPathFromCaller(t, 0, exactLedgerSnapshotDefault)
	}
	if _, err := os.Stat(sourcePath); err != nil {
		t.Skipf("exact-ledger artifact unavailable: set %s or provide %s", exactLedgerSnapshotEnv, sourcePath)
	}
	snapshotPath := copyAndVerifyExactLedger(t, sourcePath)
	snapshotPayload, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read verified exact-ledger copy: %v", err)
	}
	return snapshotPath, snapshotPayload
}

func startExactLedgerServer(t *testing.T, factoryDir, snapshotPath string, snapshotPayload []byte, successorPath string) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		ServerReadyTimeout:        90 * time.Second,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--resume", snapshotPath, "--record", successorPath},
		Edges: serviceedges.Edges{
			FactorySessionReplayRecordingReader: func(path string) ([]byte, error) {
				t.Logf("resume reader selected %q", path)
				return append([]byte(nil), snapshotPayload...), nil
			},
			ProviderCommandRunner: support.NewStaticSuccessCommandRunner("exact-ledger controlled provider output"),
			ScriptCommandRunner:   support.NewStaticSuccessCommandRunner("blocked"),
		},
	})
}

func prepareExactLedgerInitialState(t *testing.T, server *support.FunctionalAPIServer) exactLedgerInitialState {
	t.Helper()
	status := support.WaitForStatus(t, server.URL(), 90*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus != ""
	})
	t.Logf("exact resume status=%s initial=%d processing=%d terminal=%d failed=%d", status.RuntimeStatus, status.Categories.Initial, status.Categories.Processing, status.Categories.Terminal, status.Categories.Failed)
	session := support.GetDefaultSession(t, server.URL())
	if session.Id == "" || !session.IsDefault {
		t.Fatalf("resumed Factory Session = %#v, want the named default session", session)
	}
	parentBefore := support.GetDefaultSessionWorkByID(t, server.URL(), exactLedgerParentWorkID)
	historicalBefore := support.GetDefaultSessionWorkByID(t, server.URL(), exactLedgerHistoricalCycleID)
	currentBefore := support.GetDefaultSessionWorkByID(t, server.URL(), exactLedgerCurrentCycleID)
	assertExactLedgerWorkIdentity(t, parentBefore, exactLedgerParentWorkID, "project", "blocked")
	assertExactLedgerWorkIdentity(t, historicalBefore, exactLedgerHistoricalCycleID, "project-cycle", "blocked")
	assertExactLedgerWorkIdentity(t, currentBefore, exactLedgerCurrentCycleID, "project-cycle", "init")
	if len(support.FactoryRelationsValue(currentBefore.Relations)) == 0 {
		t.Fatalf("current cycle relations = %#v, want retained public relationships", currentBefore.Relations)
	}
	eventsBefore := server.GetFactoryEvents(t)
	if len(eventsBefore) == 0 {
		t.Fatal("resumed Factory Event history is empty")
	}
	return exactLedgerInitialState{
		sessionID:             session.Id,
		lastEventSequence:     eventsBefore[len(eventsBefore)-1].Context.Sequence,
		currentWorkerSessions: support.ListSessionWorkerSessions(t, server.URL(), session.Id, exactLedgerCurrentWorkID()).Sessions,
	}
}

func completeExactLedgerCurrentCycle(t *testing.T, server *support.FunctionalAPIServer, initial exactLedgerInitialState) {
	t.Helper()
	for _, dependencyID := range exactLedgerCurrentDependencyWorkIDs {
		completed := moveExactLedgerWork(t, server.URL(), dependencyID, "complete")
		if completed.State == nil || completed.State.Name != "complete" {
			t.Fatalf("dependency Work %q state = %#v, want complete", dependencyID, completed.State)
		}
	}
	waitForExactLedgerWorkerSessionCompletion(t, server.URL(), initial.sessionID, exactLedgerCurrentWorkID(), initial.currentWorkerSessions, 90*time.Second)
	currentFailed := support.GetDefaultSessionWorkByID(t, server.URL(), exactLedgerCurrentWorkID())
	assertExactLedgerWorkIdentity(t, currentFailed, exactLedgerCurrentCycleID, "project-cycle", "blocked")
	if len(support.FactoryRelationsValue(currentFailed.Relations)) == 0 {
		t.Fatalf("current failed cycle relations = %#v, want retained public relationships", currentFailed.Relations)
	}
}

func recoverExactLedgerParent(t *testing.T, server *support.FunctionalAPIServer, sessionID string) {
	t.Helper()
	parentWorkerSessionsBefore := support.ListSessionWorkerSessions(t, server.URL(), sessionID, exactLedgerParentWorkID).Sessions
	moved := moveExactLedgerWork(t, server.URL(), exactLedgerParentWorkID, "init")
	assertExactLedgerWorkIdentity(t, moved, exactLedgerParentWorkID, "project", "init")
	waitForExactLedgerWorkerSessionCompletion(t, server.URL(), sessionID, exactLedgerParentWorkID, parentWorkerSessionsBefore, 90*time.Second)
	parentAfter := support.GetDefaultSessionWorkByID(t, server.URL(), exactLedgerParentWorkID)
	assertExactLedgerWorkIdentity(t, parentAfter, exactLedgerParentWorkID, "project", "blocked")
	historicalAfter := support.GetDefaultSessionWorkByID(t, server.URL(), exactLedgerHistoricalCycleID)
	assertExactLedgerWorkIdentity(t, historicalAfter, exactLedgerHistoricalCycleID, "project-cycle", "blocked")
	if len(support.FactoryRelationsValue(historicalAfter.Relations)) == 0 {
		t.Fatalf("historical cycle relations after recovery = %#v, want retained public relationships", historicalAfter.Relations)
	}
}

func waitForExactLedgerWorkerSessionCompletion(
	t *testing.T,
	baseURL, sessionID, workID string,
	previous []factoryapi.WorkerSessionObservation,
	timeout time.Duration,
) factoryapi.WorkerSessionObservation {
	t.Helper()
	previousStates := make(map[string]factoryapi.WorkerSessionObservationState, len(previous))
	for _, observation := range previous {
		previousStates[observation.WorkerSessionId] = observation.State
	}
	observations := support.ListSessionWorkerSessions(t, baseURL, sessionID, workID).Sessions
	for _, observation := range observations {
		previousState, existed := previousStates[observation.WorkerSessionId]
		if existed && isExactLedgerWorkerSessionTerminal(previousState) {
			continue
		}
		if observation.WorkerSessionId == "" {
			continue
		}
		waitForExactLedgerWorkerSessionTerminal(t, baseURL, sessionID, observation.WorkerSessionId, timeout)
		terminal := getExactLedgerWorkerSessionByID(t, baseURL, sessionID, observation.WorkerSessionId)
		if !isExactLedgerWorkerSessionTerminal(terminal.State) {
			t.Fatalf("Worker Session %q state = %q after terminal event, want terminal", terminal.WorkerSessionId, terminal.State)
		}
		return terminal
	}
	t.Fatalf("no new public Worker Session observation was available for Work %q", workID)
	return factoryapi.WorkerSessionObservation{}
}

func waitForExactLedgerWorkerSessionTerminal(
	t *testing.T,
	baseURL, sessionID, workerSessionID string,
	timeout time.Duration,
) {
	t.Helper()
	if strings.TrimSpace(workerSessionID) == "" {
		t.Fatal("worker session id is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(sessionID) +
		"/worker-sessions/" + url.PathEscape(workerSessionID) + "/events"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build live Worker Session events request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET live Worker Session events: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET live Worker Session events status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatalf("decode live Worker Session event: %v", err)
		}
		if event.Delivery == factoryapi.WorkerSessionEventDeliverySourceFailure {
			t.Fatalf("live Worker Session event source failure: %#v", event)
		}
		if event.Delivery == factoryapi.WorkerSessionEventDeliveryTerminal ||
			event.Delivery == factoryapi.WorkerSessionEventDeliveryTerminalReplay {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read live Worker Session events: %v", err)
	}
	t.Fatalf("live Worker Session event stream ended without terminal delivery")
}

func getExactLedgerWorkerSessionByID(
	t *testing.T,
	baseURL, sessionID, workerSessionID string,
) factoryapi.WorkerSessionObservation {
	t.Helper()
	if strings.TrimSpace(workerSessionID) == "" {
		t.Fatal("worker session id is empty")
	}
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(sessionID) +
		"/worker-sessions/" + url.PathEscape(workerSessionID)
	return support.GetJSON[factoryapi.WorkerSessionObservation](t, endpoint)
}

func isExactLedgerWorkerSessionTerminal(state factoryapi.WorkerSessionObservationState) bool {
	switch state {
	case factoryapi.WorkerSessionObservationStateCompleted,
		factoryapi.WorkerSessionObservationStateFailed,
		factoryapi.WorkerSessionObservationStateCanceled,
		factoryapi.WorkerSessionObservationStateTerminated:
		return true
	default:
		return false
	}
}

func exactLedgerCurrentWorkID() string {
	return exactLedgerCurrentCycleID
}

func exactLedgerEventsAfterSequence(events []factoryapi.FactoryEvent, sequence int) []factoryapi.FactoryEvent {
	filtered := make([]factoryapi.FactoryEvent, 0)
	for _, event := range events {
		if event.Context.Sequence > sequence {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func assertExactLedgerWorkIdentity(t *testing.T, item factoryapi.Work, workID, workType, state string) {
	t.Helper()
	if support.StringPointerValue(item.WorkId) != workID {
		t.Fatalf("Work response ID = %q, want %q", support.StringPointerValue(item.WorkId), workID)
	}
	if support.StringPointerValue(item.WorkTypeName) != workType {
		t.Fatalf("Work %q type = %q, want %q", workID, support.StringPointerValue(item.WorkTypeName), workType)
	}
	if item.State == nil || item.State.Name != state {
		t.Fatalf("Work %q state = %#v, want %q", workID, item.State, state)
	}
}

func moveExactLedgerWork(t *testing.T, baseURL, workID, state string) factoryapi.Work {
	t.Helper()
	payload, err := json.Marshal(factoryapi.MoveWorkRequest{StateName: state})
	if err != nil {
		t.Fatalf("marshal exact-ledger recovery move: %v", err)
	}
	endpoint := support.DefaultSessionWorkURL(baseURL, "/work/"+url.PathEscape(workID)+"/move")
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var moved factoryapi.Work
	if err := json.NewDecoder(response.Body).Decode(&moved); err != nil {
		t.Fatalf("decode exact-ledger recovery move: %v", err)
	}
	return moved
}

func assertExactLedgerFeedback(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	dispatches := support.ObserveDispatchEvents(t, events)
	accepted := map[string]int{}
	for _, dispatch := range dispatches {
		if dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			continue
		}
		transition := dispatch.Request.TransitionId
		includesCurrent := support.DispatchObservationIncludesWork(dispatch, exactLedgerCurrentCycleID)
		includesParent := support.DispatchObservationIncludesWork(dispatch, exactLedgerParentWorkID)
		includesHistorical := support.DispatchObservationIncludesWork(dispatch, exactLedgerHistoricalCycleID)
		if includesHistorical && transition != "" {
			switch transition {
			case "decide-project-cycle", "block-project", "complete-project", "retry-project-after-cycle-failure", "escalate-project":
				t.Fatalf("accepted %s dispatch consumed historical cycle %s: %#v", transition, exactLedgerHistoricalCycleID, dispatch)
			}
		}
		switch transition {
		case "decide-project-cycle":
			if !includesCurrent {
				continue
			}
			accepted[transition]++
		case "block-project":
			if !includesCurrent && !includesParent {
				continue
			}
			accepted[transition]++
			if !includesCurrent || !includesParent {
				t.Fatalf("accepted %s dispatch did not bind current cycle and parent: %#v", transition, dispatch)
			}
		case "escalate-project":
			if !includesParent {
				continue
			}
			accepted[transition]++
		case "complete-project", "retry-project-after-cycle-failure":
			if includesCurrent || includesParent {
				t.Fatalf("accepted %s dispatch touched the recovered current route: %#v", transition, dispatch)
			}
		}
	}
	for _, transition := range []string{"decide-project-cycle", "block-project", "escalate-project"} {
		if accepted[transition] != 1 {
			t.Fatalf("accepted %s dispatches = %d, want exactly one; events=%d", transition, accepted[transition], len(events))
		}
	}
}

func copyAndVerifyExactLedger(t *testing.T, sourcePath string) string {
	t.Helper()
	input, err := os.Open(sourcePath)
	if err != nil {
		t.Fatalf("open exact-ledger source: %v", err)
	}
	defer input.Close()

	destination := filepath.Join(t.TempDir(), "live-final-sol-snapshot.jsonl")
	output, err := os.Create(destination)
	if err != nil {
		t.Fatalf("create exact-ledger copy: %v", err)
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		t.Fatalf("copy exact-ledger source: %v", err)
	}
	if err := output.Close(); err != nil {
		t.Fatalf("close exact-ledger copy: %v", err)
	}

	verifyExactLedgerIdentity(t, destination)
	return destination
}

func verifyExactLedgerIdentity(t *testing.T, path string) {
	t.Helper()
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat exact-ledger artifact: %v", err)
	}
	hashFile, err := os.Open(path)
	if err != nil {
		t.Fatalf("open exact-ledger artifact for hash: %v", err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, hashFile); err != nil {
		hashFile.Close()
		t.Fatalf("hash exact-ledger artifact: %v", err)
	}
	if err := hashFile.Close(); err != nil {
		t.Fatalf("close exact-ledger artifact hash input: %v", err)
	}
	gotHash := strings.ToUpper(hex.EncodeToString(hash.Sum(nil)))
	if stat.Size() != exactLedgerSnapshotBytes || gotHash != exactLedgerSnapshotSHA256 {
		t.Fatalf("exact-ledger artifact identity = (%d bytes, %s), want (%d bytes, %s)", stat.Size(), gotHash, exactLedgerSnapshotBytes, exactLedgerSnapshotSHA256)
	}
}

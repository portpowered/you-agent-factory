package loop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPackagedLoopUsesInvocationDurationAndSkipsOverlap(t *testing.T) {
	start := time.Date(2026, time.July, 29, 20, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	runner := newBlockingLoopRunner()
	submissions := make(chan work.FactorySubmissionRecord, 16)
	factoryDir := support.InstallPackagedFactory(t, t.TempDir(), factorydefinitions.PackagedLoopFactoryName)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, WaitForServiceModeRuntime: true,
		Args: []string{"--provider", "CODEX", "--model", "operator-model"},
		Edges: serviceedges.Edges{
			Clock: fakeClock, ProviderCommandRunner: runner,
			SubmissionRecorder: func(record work.FactorySubmissionRecord) { submissions <- record },
		},
	})

	response := invokeLoop(t, server, map[string]any{
		"request": "check dependency updates", "every": "1m",
		"triggerAtStart": "true", "maxConsecutiveFailures": "0",
	})
	if response.Status != factoryapi.InvocationTerminalStatusTimedOut {
		t.Fatalf("invocation status = %q, want TIMED_OUT for long-lived controller", response.Status)
	}

	first := waitForLoopSubmission(t, submissions, "scheduled-execution")
	assertLoopSubmission(t, first, "init", "SCHEDULED", "1", start, start)
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("loop executor did not begin trigger-at-start execution")
	}

	waitForLoopClockWaiter(t, fakeClock)
	fakeClock.Advance(time.Minute)
	skipped := waitForLoopSubmission(t, submissions, "scheduled-execution")
	assertLoopSubmission(t, skipped, "skipped", "SKIPPED_OVERLAP", "2", start.Add(time.Minute), start.Add(time.Minute))
	if runner.calls() != 1 {
		t.Fatalf("overlapping executor calls = %d, want 1", runner.calls())
	}

	close(runner.release)
	waitForLoopDispatchCompletion(t, server)
	waitForLoopClockWaiter(t, fakeClock)
	fakeClock.Advance(time.Minute)
	recovered := waitForLoopSubmission(t, submissions, "scheduled-execution")
	assertLoopSubmission(t, recovered, "init", "SCHEDULED", "3", start.Add(2*time.Minute), start.Add(2*time.Minute))
}

func TestPackagedLoopRejectsInvalidDurationBeforeWorkAdmission(t *testing.T) {
	submissions := make(chan work.FactorySubmissionRecord, 2)
	factoryDir := support.InstallPackagedFactory(t, t.TempDir(), factorydefinitions.PackagedLoopFactoryName)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, WaitForServiceModeRuntime: true,
		UseMockWorkers: true,
		Edges:          serviceedges.Edges{SubmissionRecorder: func(record work.FactorySubmissionRecord) { submissions <- record }},
	})

	status := postLoopInvocation(t, server, map[string]any{"request": "check", "every": "tomorrow"}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid duration status = %d, want 400", status)
	}
	select {
	case record := <-submissions:
		t.Fatalf("invalid duration admitted Work = %#v", record)
	default:
	}
}

type blockingLoopRunner struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	count   int
}

func newBlockingLoopRunner() *blockingLoopRunner {
	return &blockingLoopRunner{started: make(chan struct{}), release: make(chan struct{})}
}

func (runner *blockingLoopRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.count++
	runner.mu.Unlock()
	runner.once.Do(func() { close(runner.started) })
	select {
	case <-runner.release:
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("scheduled execution complete")}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func (runner *blockingLoopRunner) calls() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.count
}

func invokeLoop(t *testing.T, server *support.FunctionalAPIServer, args map[string]any) factoryapi.InvocationResponse {
	t.Helper()
	timeoutMillis := int64(20)
	var response factoryapi.InvocationResponse
	status := postLoopInvocation(t, server, args, func(decoded factoryapi.InvocationResponse) { response = decoded }, &timeoutMillis)
	if status != http.StatusOK {
		t.Fatalf("loop invocation status = %d, want 200", status)
	}
	return response
}

func postLoopInvocation(
	t *testing.T,
	server *support.FunctionalAPIServer,
	args map[string]any,
	decode func(factoryapi.InvocationResponse),
	timeout ...*int64,
) int {
	t.Helper()
	requestID := fmt.Sprintf("packaged-loop-%d", time.Now().UnixNano())
	request := factoryapi.InvocationRequest{RequestId: &requestID, Args: &args}
	if len(timeout) > 0 {
		request.TimeoutMillis = timeout[0]
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal loop invocation: %v", err)
	}
	response, err := http.Post(server.URL()+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST loop invocation: %v", err)
	}
	defer response.Body.Close()
	if decode != nil && response.StatusCode == http.StatusOK {
		var decoded factoryapi.InvocationResponse
		if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
			t.Fatalf("decode loop invocation: %v", err)
		}
		decode(decoded)
	}
	return response.StatusCode
}

func waitForLoopSubmission(t *testing.T, submissions <-chan work.FactorySubmissionRecord, workType string) work.FactorySubmissionRecord {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case record := <-submissions:
			if record.Request.WorkTypeID == workType {
				return record
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s submission", workType)
		}
	}
}

func assertLoopSubmission(
	t *testing.T,
	record work.FactorySubmissionRecord,
	wantState, wantOutcome, wantSequence string,
	wantNominal, wantActual time.Time,
) {
	t.Helper()
	request := record.Request
	if request.TargetState != wantState {
		t.Fatalf("scheduled state = %q, want %q", request.TargetState, wantState)
	}
	if request.Tags["agent_factory.time.trigger_outcome"] != wantOutcome || request.Tags["agent_factory.time.sequence"] != wantSequence {
		t.Fatalf("scheduled outcome tags = %#v", request.Tags)
	}
	if request.Tags[factorydefinitions.TimeWorkTagKeyNominalAt] != wantNominal.Format(time.RFC3339Nano) ||
		request.Tags["agent_factory.time.actual_at"] != wantActual.Format(time.RFC3339Nano) {
		t.Fatalf("scheduled timing tags = %#v", request.Tags)
	}
}

func waitForLoopClockWaiter(t *testing.T, clock *clockwork.FakeClock) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := clock.BlockUntilContext(ctx, 1); err != nil {
		t.Fatalf("wait for loop scheduler timer: %v", err)
	}
}

func waitForLoopDispatchCompletion(t *testing.T, server *support.FunctionalAPIServer) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
		if len(dispatches) > 0 && dispatches[len(dispatches)-1].Response != nil {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("timed out waiting for loop executor completion")
}

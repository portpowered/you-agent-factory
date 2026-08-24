package stress_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const runtimeReadMetricName = "factory_runtime.read.observation"

// TestFactorySessionReadScaleProfile drives the real process and public HTTP
// surface while one non-board Work item grows retained event history. The
// operation labels, rather than machine-sensitive timings, are the oracle.
func TestFactorySessionReadScaleProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping retained-history Factory Session read scale profile in short mode")
	}

	const workerName = "session-read-scale-worker"
	checkpoints := []int{1, 50, 200, 1600}
	dir := testutil.ScaffoldFactoryDir(t, sessionReadScaleConfig(workerName))
	readMetrics := newFactorySessionReadMetricsRecorder()
	mux := workerMuxExecutor{}
	harness := startStressProcessWithMetrics(
		t,
		dir,
		workerExecutorProvider{executor: mux},
		readMetrics,
	)
	harness.workers = mux
	executor := newSessionReadScaleDispatchExecutor(checkpoints)
	harness.SetWorkerExecutor(workerName, executor)
	defaultSession := harness.Session()
	if defaultSession.Id == "" {
		t.Fatal("default live Factory Session has empty identity")
	}
	defer func() {
		// Cancel the Process first so in-flight worker calls observe the same
		// shutdown authority as the engine. The executor stop then releases any
		// test-only gate before we join Process.Execute below.
		harness.cancel()
		executor.stop()
		stopSessionReadScaleProcess(harness)
	}()

	requests := []work.SubmitRequest{{WorkID: "session-read-review", WorkTypeID: "review", Name: "review", Payload: []byte(`{"kind":"approval"}`)}}
	for index := range 8 {
		requests = append(requests, work.SubmitRequest{
			WorkID: fmt.Sprintf("session-read-loop-%d", index), WorkTypeID: "task",
			Name: fmt.Sprintf("loop-%d", index), TargetState: "loop", Payload: []byte(`{"kind":"growth"}`),
		})
	}
	harness.SubmitFull(t.Context(), requests)

	var baseline factorySessionReadOperationCounts
	previousRetainedEvents := 0
	approvalID := ""
	for index, checkpoint := range checkpoints {
		waitForSessionReadScaleCheckpoint(t, executor, checkpoint)
		retainedEvents := measureWorkListEventSubscription(t, harness).retainedEvents
		if retainedEvents <= previousRetainedEvents {
			previousCheckpoint := 0
			if index > 0 {
				previousCheckpoint = checkpoints[index-1]
			}
			t.Fatalf("checkpoint %d retained events = %d, want more than checkpoint %d's %d", checkpoint, retainedEvents, previousCheckpoint, previousRetainedEvents)
		}
		previousRetainedEvents = retainedEvents

		approvalID = readPendingApproval(t, harness, approvalID)
		listOperations := readFactorySessionList(t, harness, readMetrics, defaultSession.Id, approvalID)
		detailOperations := readFactorySessionDetail(t, harness, readMetrics, defaultSession.Id, approvalID)
		statusOperations := readFactorySessionStatus(t, harness, readMetrics)

		operations := factorySessionReadOperationCounts{
			list:   listOperations,
			detail: detailOperations,
			status: statusOperations,
		}
		t.Logf("retained-history checkpoint=%d retained_events=%d list=%+v detail=%+v status=%+v", checkpoint, retainedEvents, operations.list, operations.detail, operations.status)
		if index == 0 {
			baseline = operations
		} else if operations != baseline {
			t.Fatalf("checkpoint %d read operation counts = %+v, want flat baseline %+v", checkpoint, operations, baseline)
		}
		if index < len(checkpoints)-1 {
			executor.releaseCheckpoint(checkpoint)
		}
	}
}

func stopSessionReadScaleProcess(harness *stressProcessHarness) {
	if harness == nil || harness.cancel == nil {
		return
	}
	harness.cancel()
	select {
	case err := <-harness.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			harness.t.Errorf("Process.Execute() shutdown error = %v", err)
		}
	case <-time.After(15 * time.Second):
		harness.t.Errorf("timed out waiting for Process.Execute() shutdown")
	}
	harness.cancel = nil
}

func waitForSessionReadScaleCheckpoint(t *testing.T, executor *sessionReadScaleDispatchExecutor, target int) {
	t.Helper()
	executor.mu.Lock()
	checkpoint := executor.checkpoints[target]
	executor.mu.Unlock()
	if checkpoint == nil {
		t.Fatalf("dispatch executor has no checkpoint %d", target)
	}
	select {
	case <-checkpoint.reached:
	case <-time.After(2 * time.Minute):
		t.Fatalf("timed out waiting for retained-history checkpoint %d", target)
	}
}

type sessionReadScaleDispatchCheckpoint struct {
	reached     chan struct{}
	reachedOnce sync.Once
	release     chan struct{}
	releaseOnce sync.Once
}

type sessionReadScaleDispatchExecutor struct {
	mu          sync.Mutex
	checkpoints map[int]*sessionReadScaleDispatchCheckpoint
	ordered     []int
	target      int
	calls       int
	stopped     bool
	active      sync.WaitGroup
	stopCh      chan struct{}
	stopOnce    sync.Once
}

func newSessionReadScaleDispatchExecutor(checkpoints []int) *sessionReadScaleDispatchExecutor {
	result := &sessionReadScaleDispatchExecutor{
		checkpoints: make(map[int]*sessionReadScaleDispatchCheckpoint, len(checkpoints)),
		ordered:     append([]int(nil), checkpoints...),
		target:      checkpoints[len(checkpoints)-1],
		stopCh:      make(chan struct{}),
	}
	for _, checkpoint := range checkpoints {
		result.checkpoints[checkpoint] = &sessionReadScaleDispatchCheckpoint{
			reached: make(chan struct{}), release: make(chan struct{}),
		}
	}
	return result
}

func (executor *sessionReadScaleDispatchExecutor) Execute(ctx context.Context, dispatch work.WorkDispatch) (workers.WorkResult, error) {
	executor.mu.Lock()
	executor.calls++
	call := executor.calls
	if executor.stopped {
		executor.mu.Unlock()
		return sessionReadScaleCancellation(dispatch), nil
	}
	executor.active.Add(1)
	defer executor.active.Done()
	var checkpoint *sessionReadScaleDispatchCheckpoint
	for _, target := range executor.ordered {
		if call < target {
			break
		}
		candidate := executor.checkpoints[target]
		select {
		case <-candidate.release:
		default:
			checkpoint = candidate
		}
		if checkpoint != nil {
			break
		}
	}
	executor.mu.Unlock()
	if checkpoint != nil {
		checkpoint.reachedOnce.Do(func() { close(checkpoint.reached) })
		select {
		case <-checkpoint.release:
		case <-ctx.Done():
			return workers.WorkResult{}, ctx.Err()
		case <-executor.stopCh:
			return sessionReadScaleCancellation(dispatch), nil
		}
	}
	if call > executor.target {
		select {
		case <-ctx.Done():
			return workers.WorkResult{}, ctx.Err()
		case <-executor.stopCh:
			return sessionReadScaleCancellation(dispatch), nil
		}
	}
	return workers.WorkResult{
		DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID,
		Outcome: workers.OutcomeAccepted,
	}, nil
}

func sessionReadScaleCancellation(dispatch work.WorkDispatch) workers.WorkResult {
	return workers.WorkResult{
		DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID,
		Outcome: workers.OutcomeCanceled,
		Cancellation: &workers.DispatchCancellation{
			Reason: workers.DispatchCancellationReasonCanceled,
		},
	}
}

func (executor *sessionReadScaleDispatchExecutor) stop() {
	executor.stopOnce.Do(func() {
		executor.mu.Lock()
		executor.stopped = true
		close(executor.stopCh)
		executor.mu.Unlock()
		executor.active.Wait()
	})
}

func (executor *sessionReadScaleDispatchExecutor) releaseCheckpoint(target int) {
	executor.mu.Lock()
	checkpoint := executor.checkpoints[target]
	executor.mu.Unlock()
	if checkpoint != nil {
		checkpoint.releaseOnce.Do(func() { close(checkpoint.release) })
	}
}

type factorySessionReadOperation struct {
	canonicalHistoryVisits int
	canonicalEventsCopied  int
	fullHistoryReductions  int
	runtimeSnapshotReads   int
	operationCount         int
}

type factorySessionReadOperationCounts struct {
	list   factorySessionReadOperation
	detail factorySessionReadOperation
	status factorySessionReadOperation
}

type factorySessionReadMetricsRecorder struct {
	mu      sync.Mutex
	metrics []factorysessions.InvocationMetric
}

func newFactorySessionReadMetricsRecorder() *factorySessionReadMetricsRecorder {
	return &factorySessionReadMetricsRecorder{}
}

func (r *factorySessionReadMetricsRecorder) RecordInvocationMetric(metric factorysessions.InvocationMetric) {
	if r == nil || metric.Name != runtimeReadMetricName {
		return
	}
	labels := make(map[string]string, len(metric.Labels))
	for key, value := range metric.Labels {
		labels[key] = value
	}
	r.mu.Lock()
	r.metrics = append(r.metrics, factorysessions.InvocationMetric{Name: metric.Name, Labels: labels})
	r.mu.Unlock()
}

func (r *factorySessionReadMetricsRecorder) reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.metrics = nil
	r.mu.Unlock()
}

func (r *factorySessionReadMetricsRecorder) take(t *testing.T, endpoint string) factorySessionReadOperation {
	t.Helper()
	r.mu.Lock()
	metrics := append([]factorysessions.InvocationMetric(nil), r.metrics...)
	r.metrics = nil
	r.mu.Unlock()
	if len(metrics) != 1 {
		t.Fatalf("%s runtime read metrics = %d, want one", endpoint, len(metrics))
	}
	metric := metrics[0]
	return factorySessionReadOperation{
		canonicalHistoryVisits: factorySessionReadMetricInt(t, metric, endpoint, "canonical_history_visits"),
		canonicalEventsCopied:  factorySessionReadMetricInt(t, metric, endpoint, "canonical_events_copied"),
		fullHistoryReductions:  factorySessionReadMetricInt(t, metric, endpoint, "full_history_reductions"),
		runtimeSnapshotReads:   factorySessionReadMetricInt(t, metric, endpoint, "runtime_snapshot_reads"),
		operationCount:         factorySessionReadMetricInt(t, metric, endpoint, "operation_count"),
	}
}

func factorySessionReadMetricInt(t *testing.T, metric factorysessions.InvocationMetric, endpoint, name string) int {
	t.Helper()
	var value int
	if _, err := fmt.Sscanf(metric.Labels[name], "%d", &value); err != nil || value < 0 {
		t.Fatalf("%s runtime read metric %q = %q, want non-negative integer", endpoint, name, metric.Labels[name])
	}
	return value
}

func readFactorySessionList(
	t *testing.T,
	harness *stressProcessHarness,
	metrics *factorySessionReadMetricsRecorder,
	wantSessionID string,
	wantApprovalID string,
) factorySessionReadOperation {
	t.Helper()
	metrics.reset()
	var response factoryapi.ListFactorySessionsResponse
	getFactorySessionReadJSON(t, harness.baseURL+"/factory-sessions", &response)
	if len(response.Sessions) != 1 || response.Sessions[0].Id != wantSessionID {
		t.Fatalf("live Factory Session list = %#v, want session %q", response.Sessions, wantSessionID)
	}
	assertPendingSessionApproval(t, response.Sessions[0].Runtime, wantApprovalID)
	return metrics.take(t, "GET /factory-sessions")
}

func readFactorySessionDetail(
	t *testing.T,
	harness *stressProcessHarness,
	metrics *factorySessionReadMetricsRecorder,
	wantSessionID string,
	wantApprovalID string,
) factorySessionReadOperation {
	t.Helper()
	metrics.reset()
	var response factoryapi.FactorySessionGetResponse
	getFactorySessionReadJSON(t, harness.sessionURL(), &response)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode Factory Session detail: %v", err)
	}
	if session.Id != wantSessionID {
		t.Fatalf("Factory Session detail id = %q, want %q", session.Id, wantSessionID)
	}
	assertPendingSessionApproval(t, &session.Runtime, wantApprovalID)
	return metrics.take(t, "GET /factory-sessions/{id}")
}

func readFactorySessionStatus(
	t *testing.T,
	harness *stressProcessHarness,
	metrics *factorySessionReadMetricsRecorder,
) factorySessionReadOperation {
	t.Helper()
	metrics.reset()
	var response factoryapi.StatusResponse
	getFactorySessionReadJSON(t, harness.baseURL+"/factory-sessions/"+factorysessions.DefaultSessionID+"/status", &response)
	if strings.TrimSpace(response.FactoryState) == "" || strings.TrimSpace(response.RuntimeStatus) == "" {
		t.Fatalf("Factory Session status = %#v, want runtime state and status", response)
	}
	return metrics.take(t, "GET /factory-sessions/{id}/status")
}

func readPendingApproval(t *testing.T, harness *stressProcessHarness, wantApprovalID string) string {
	t.Helper()
	var response factoryapi.ListHumanApprovalsResponse
	getFactorySessionReadJSON(t, harness.baseURL+"/factory-sessions/"+factorysessions.DefaultSessionID+"/approvals", &response)
	if len(response.Approvals) != 1 {
		t.Fatalf("pending approval read = %#v, want one approval", response.Approvals)
	}
	approvalID := response.Approvals[0].ApprovalId
	if wantApprovalID != "" && approvalID != wantApprovalID {
		t.Fatalf("pending approval identity changed from %q to %q", wantApprovalID, approvalID)
	}
	return approvalID
}

func assertPendingSessionApproval(t *testing.T, runtime *factoryapi.FactorySessionRuntime, wantApprovalID string) {
	t.Helper()
	if runtime == nil || runtime.PendingHumanApprovals == nil || len(*runtime.PendingHumanApprovals) != 1 || (*runtime.PendingHumanApprovals)[0].ApprovalId != wantApprovalID {
		t.Fatalf("Factory Session pending approvals = %#v, want approval %q", runtime, wantApprovalID)
	}
}

func getFactorySessionReadJSON(t *testing.T, endpoint string, target any) {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		t.Fatalf("GET %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode GET %s: %v", endpoint, err)
	}
}

func sessionReadScaleConfig(workerName string) *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "loop", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
			{
				Name: "review",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "approved", Type: interfaces.StateTypeTerminal},
					{Name: "rejected", Type: interfaces.StateTypeProcessing},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workers: []interfaces.FactoryWorkerConfig{{Name: workerName}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "loop",
				WorkerTypeName: workerName,
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "loop"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "loop"}},
				OnFailure:      []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
			},
			{
				ID:   "session-read-review",
				Name: "review",
				Type: interfaces.WorkstationTypeHumanApproval,
				Description: &interfaces.NameValueConfig{
					Type:  interfaces.NameValueTypeLocalizableAsset,
					Value: "Review the stress item",
				},
				Inputs:      []interfaces.IOConfig{{WorkTypeName: "review", StateName: "init"}},
				Outputs:     []interfaces.IOConfig{{WorkTypeName: "review", StateName: "approved"}},
				OnRejection: []interfaces.IOConfig{{WorkTypeName: "review", StateName: "rejected"}},
			},
		},
	}
}

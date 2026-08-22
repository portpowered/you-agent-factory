package stress_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	recordingshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	workListProfileRows                  = 169
	workListProfilePageSize              = 50
	workListProfileWorkerLoad            = 27
	workListProfileAccumulatedDispatches = 200
	workListProfileTimeout               = 30 * time.Second
	workListProfilePageLimit             = 500 * time.Millisecond
	workListProfileWalkLimit             = 2 * time.Second
	workListStabilityRounds              = 2
)

// TestWorkListLatencyProfile characterizes the incident-sized Work list
// operation through the real HTTP server. The default run reports a baseline
// without making a machine-sensitive performance claim. Set
// YOU_WORK_LIST_LATENCY_ENFORCE=1 to turn the measured target into an
// opt-in regression assertion; that command is intentionally expected to fail
// on the parent implementation until the read-path bottleneck is removed.
func TestWorkListLatencyProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping incident-sized Work list latency profile in short mode")
	}

	for _, workerLoad := range []int{0, workListProfileWorkerLoad} {
		workerLoad := workerLoad
		t.Run(fmt.Sprintf("workers-%d", workerLoad), func(t *testing.T) {
			profile := runWorkListLatencyProfile(t, workerLoad)
			logWorkListLatencyProfile(t, profile)
			if os.Getenv("YOU_WORK_LIST_LATENCY_ENFORCE") == "1" {
				assertWorkListLatencyTarget(t, profile)
			}
		})
	}
	t.Run("accumulated-events", func(t *testing.T) {
		profile := runAccumulatedWorkListLatencyProfile(t)
		logWorkListLatencyProfile(t, profile)
		if profile.retainedEvents <= profile.accumulatedDispatches {
			t.Fatalf("event-growth profile retained %d events after %d dispatches; want history growth", profile.retainedEvents, profile.accumulatedDispatches)
		}
		if os.Getenv("YOU_WORK_LIST_LATENCY_ENFORCE") == "1" {
			assertWorkListLatencyTarget(t, profile)
		}
	})
}

// TestWorkListLatencyStability repeats complete public pagination walks while
// the identified degradation dimensions are present. The default run reports
// every sample; YOU_WORK_LIST_LATENCY_ENFORCE=1 applies the incident bounds to
// each sample as an opt-in, machine-sensitive regression assertion.
func TestWorkListLatencyStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping repeated Work list stability profile in short mode")
	}
	enforceTarget := os.Getenv("YOU_WORK_LIST_LATENCY_ENFORCE") == "1"

	t.Run("worker-contention", func(t *testing.T) {
		for _, workerLoad := range []int{0, workListProfileWorkerLoad} {
			workerLoad := workerLoad
			t.Run(fmt.Sprintf("workers-%d", workerLoad), func(t *testing.T) {
				samples := runWorkerContentionStability(t, workerLoad)
				assertWorkListStabilitySamples(t, samples, enforceTarget)
			})
		}
	})

	t.Run("event-growth", func(t *testing.T) {
		samples := runEventGrowthStability(t)
		assertWorkListStabilitySamples(t, samples, enforceTarget)
		for index := 1; index < len(samples); index++ {
			if samples[index].retainedEvents <= samples[index-1].retainedEvents {
				t.Fatalf(
					"event-growth sample %d retained %d events after %d dispatches; want more than sample %d's %d events",
					index+1,
					samples[index].retainedEvents,
					samples[index].accumulatedDispatches,
					index,
					samples[index-1].retainedEvents,
				)
			}
		}
	})
}

type workListLatencyProfile struct {
	scenario              string
	boardRows             int
	workerLoad            int
	accumulatedDispatches int
	retainedEvents        int
	snapshotRead          phaseSample
	eventSubscription     phaseSample
	pages                 []workListProfilePage
	totalWalk             time.Duration
	returnedWorkIDs       []string
	expectedWorkIDs       []string
	operationEstimates    workListOperationEstimates
}

type phaseSample struct {
	duration time.Duration
	bytes    int
}

type workListProfilePage struct {
	number    int
	duration  time.Duration
	bytes     int
	resultIDs []string
	nextToken string
}

type workListOperationEstimates struct {
	snapshotReads int
	// Work admission history is session-projected, so page reads do not open
	// event subscriptions or visit the retained event prefix.
	eventSubscriptions  int
	eventRecordsVisited int
	workRowsProjected   int
}

type workListStabilitySample struct {
	scenario              string
	round                 int
	boardRows             int
	workerLoad            int
	accumulatedDispatches int
	retainedEvents        int
	updatesDuringWalk     bool
	eventSubscription     phaseSample
	pages                 []workListProfilePage
	totalWalk             time.Duration
	returnedWorkIDs       []string
	expectedWorkIDs       []string
	operationEstimates    workListOperationEstimates
}

func runWorkListLatencyProfile(t *testing.T, workerLoad int) workListLatencyProfile {
	t.Helper()

	const workerName = "work-list-profile-worker"
	dir := testutil.ScaffoldFactoryDir(t, testutil.PipelineConfig(1, workerName))
	harness := startStressProcessWithWorkerMux(t, dir)
	var executor *queryLatencyExecutor
	if workerLoad > 0 {
		executor = newQueryLatencyExecutor(workListProfileRows)
		harness.SetWorkerExecutor(workerName, executor)
	}
	defer func() {
		if executor != nil {
			executor.release()
			harness.WaitForTerminalCount(workListProfileRows, queryLatencyWait)
		}
		harness.Stop()
	}()

	expectedIDs := workListProfileRequests(workListProfileRows, workerLoad)
	harness.SubmitFull(t.Context(), expectedIDs)
	if workerLoad > 0 {
		executor.waitForStarts(t, workerLoad)
	} else {
		harness.WaitForTerminalCount(workListProfileRows, queryLatencyWait)
	}

	eventSubscription := measureWorkListEventSubscription(t, harness)
	snapshotRead := measureWorkListSessionRead(t, harness)
	pages, totalWalk, returnedIDs := walkWorkListPages(t, harness, expectedIDs, workerLoad)

	return workListLatencyProfile{
		scenario:          "worker-contention",
		boardRows:         len(returnedIDs),
		workerLoad:        workerLoad,
		retainedEvents:    eventSubscription.retainedEvents,
		snapshotRead:      snapshotRead,
		eventSubscription: phaseSample{duration: eventSubscription.duration, bytes: eventSubscription.bytes},
		pages:             pages,
		totalWalk:         totalWalk,
		returnedWorkIDs:   returnedIDs,
		expectedWorkIDs:   submitRequestWorkIDs(expectedIDs),
		operationEstimates: workListOperationEstimates{
			snapshotReads:     len(pages),
			workRowsProjected: len(returnedIDs),
		},
	}
}

func runAccumulatedWorkListLatencyProfile(t *testing.T) workListLatencyProfile {
	t.Helper()

	const (
		workerName = "work-list-profile-loop-worker"
		loopState  = "loop"
	)
	dir := testutil.ScaffoldFactoryDir(t, accumulatedWorkListProfileConfig(workerName))
	harness := startStressProcessWithWorkerMux(t, dir)
	executor := newAccumulatedDispatchExecutor(workListProfileAccumulatedDispatches)
	harness.SetWorkerExecutor(workerName, executor)
	defer func() {
		executor.release()
		harness.WaitForTerminalCount(workListProfileRows, queryLatencyWait)
		harness.Stop()
	}()

	expectedRequests := accumulatedWorkListProfileRequests(workListProfileRows, loopState)
	harness.SubmitFull(t.Context(), expectedRequests)
	executor.waitForTarget(t)

	eventSubscription := measureWorkListEventSubscription(t, harness)
	snapshotRead := measureWorkListSessionRead(t, harness)
	pages, totalWalk, returnedIDs := walkWorkListPages(t, harness, expectedRequests, 1)

	return workListLatencyProfile{
		scenario:              "event-growth",
		boardRows:             len(returnedIDs),
		workerLoad:            1,
		accumulatedDispatches: workListProfileAccumulatedDispatches,
		retainedEvents:        eventSubscription.retainedEvents,
		snapshotRead:          snapshotRead,
		eventSubscription:     phaseSample{duration: eventSubscription.duration, bytes: eventSubscription.bytes},
		pages:                 pages,
		totalWalk:             totalWalk,
		returnedWorkIDs:       returnedIDs,
		expectedWorkIDs:       submitRequestWorkIDs(expectedRequests),
		operationEstimates: workListOperationEstimates{
			snapshotReads:     len(pages),
			workRowsProjected: len(returnedIDs),
		},
	}
}

func runWorkerContentionStability(t *testing.T, workerLoad int) []workListStabilitySample {
	t.Helper()

	const workerName = "work-list-stability-worker"
	dir := testutil.ScaffoldFactoryDir(t, testutil.PipelineConfig(1, workerName))
	harness := startStressProcessWithWorkerMux(t, dir)
	var executor *queryLatencyExecutor
	if workerLoad > 0 {
		executor = newQueryLatencyExecutor(workListProfileRows)
		harness.SetWorkerExecutor(workerName, executor)
	}
	defer func() {
		if executor != nil {
			executor.release()
			harness.WaitForTerminalCount(workListProfileRows, queryLatencyWait)
		}
		harness.Stop()
	}()

	expectedRequests := workListProfileRequests(workListProfileRows, workerLoad)
	harness.SubmitFull(t.Context(), expectedRequests)
	if workerLoad > 0 {
		executor.waitForStarts(t, workerLoad)
	} else {
		harness.WaitForTerminalCount(workListProfileRows, queryLatencyWait)
	}
	// Prime the real HTTP/read path before collecting machine-sensitive samples.
	// This is an observed request, not time-based synchronization; readiness is
	// established by the completed public census itself.
	walkWorkListPages(t, harness, expectedRequests, workerLoad)
	return repeatWorkListWalks(t, harness, expectedRequests, workerLoad, "worker-contention", workListStabilityRounds)
}

func runEventGrowthStability(t *testing.T) []workListStabilitySample {
	t.Helper()

	const (
		workerName = "work-list-stability-loop-worker"
		loopState  = "loop"
	)
	checkpoints := []int{1, 50, 100, workListProfileAccumulatedDispatches}
	dir := testutil.ScaffoldFactoryDir(t, accumulatedWorkListProfileConfig(workerName))
	harness := startStressProcessWithWorkerMux(t, dir)
	executor := newCheckpointedDispatchExecutor(checkpoints)
	harness.SetWorkerExecutor(workerName, executor)
	defer func() {
		executor.releaseAll()
		harness.WaitForTerminalCount(workListProfileRows, queryLatencyWait)
		harness.Stop()
	}()

	expectedRequests := accumulatedWorkListProfileRequests(workListProfileRows, loopState)
	harness.SubmitFull(t.Context(), expectedRequests)
	samples := make([]workListStabilitySample, 0, len(checkpoints))
	for _, dispatches := range checkpoints {
		executor.waitForCheckpoint(t, dispatches)
		// Warm each event-history level through the public endpoint before
		// recording its repeated-read sample.
		walkWorkListPages(t, harness, expectedRequests, 1)
		eventSubscription := measureWorkListEventSubscription(t, harness)
		updatesDuringWalk := dispatches != checkpoints[len(checkpoints)-1]
		pages, totalWalk, returnedIDs := walkWorkListPagesWithHook(
			t,
			harness,
			expectedRequests,
			1,
			func(pageNumber int) {
				if pageNumber == 1 && updatesDuringWalk {
					executor.releaseCheckpoint(dispatches)
				}
			},
		)
		samples = append(samples, workListStabilitySample{
			scenario:              "event-growth",
			round:                 len(samples) + 1,
			boardRows:             len(returnedIDs),
			workerLoad:            1,
			accumulatedDispatches: dispatches,
			retainedEvents:        eventSubscription.retainedEvents,
			updatesDuringWalk:     updatesDuringWalk,
			eventSubscription:     eventSubscription.phaseSample,
			pages:                 pages,
			totalWalk:             totalWalk,
			returnedWorkIDs:       returnedIDs,
			expectedWorkIDs:       expectedWorkListOrder(expectedRequests, 1),
			operationEstimates: workListOperationEstimates{
				snapshotReads:     len(pages),
				workRowsProjected: len(returnedIDs),
			},
		})
	}
	return samples
}

func repeatWorkListWalks(
	t *testing.T,
	harness *stressProcessHarness,
	expectedRequests []work.SubmitRequest,
	workerLoad int,
	scenario string,
	rounds int,
) []workListStabilitySample {
	t.Helper()
	samples := make([]workListStabilitySample, 0, rounds)
	expectedWorkIDs := expectedWorkListOrder(expectedRequests, workerLoad)
	for round := 1; round <= rounds; round++ {
		eventSubscription := measureWorkListEventSubscription(t, harness)
		pages, totalWalk, returnedIDs := walkWorkListPages(t, harness, expectedRequests, workerLoad)
		samples = append(samples, workListStabilitySample{
			scenario:          scenario,
			round:             round,
			boardRows:         len(returnedIDs),
			workerLoad:        workerLoad,
			retainedEvents:    eventSubscription.retainedEvents,
			eventSubscription: eventSubscription.phaseSample,
			pages:             pages,
			totalWalk:         totalWalk,
			returnedWorkIDs:   returnedIDs,
			expectedWorkIDs:   expectedWorkIDs,
			operationEstimates: workListOperationEstimates{
				snapshotReads:     len(pages),
				workRowsProjected: len(returnedIDs),
			},
		})
	}
	return samples
}

func assertWorkListStabilitySamples(t *testing.T, samples []workListStabilitySample, enforceTarget bool) {
	t.Helper()
	if len(samples) == 0 {
		t.Fatal("Work list stability profile returned no samples")
	}
	baselineIDs := samples[0].returnedWorkIDs
	baselineEstimates := samples[0].operationEstimates
	for _, sample := range samples {
		logWorkListStabilitySample(t, sample)
		if sample.boardRows != workListProfileRows {
			t.Errorf("%s round %d returned %d Work rows, want %d", sample.scenario, sample.round, sample.boardRows, workListProfileRows)
		}
		if !slices.Equal(sample.returnedWorkIDs, baselineIDs) {
			t.Errorf("%s round %d Work census changed; first=%v current=%v", sample.scenario, sample.round, baselineIDs, sample.returnedWorkIDs)
		}
		if !slices.Equal(sample.returnedWorkIDs, sample.expectedWorkIDs) {
			t.Errorf("%s round %d returned Work IDs different from expected order", sample.scenario, sample.round)
		}
		if sample.operationEstimates != baselineEstimates {
			t.Errorf("%s round %d operation estimates = %+v, want stable %+v", sample.scenario, sample.round, sample.operationEstimates, baselineEstimates)
		}
		if sample.operationEstimates.eventSubscriptions != 0 || sample.operationEstimates.eventRecordsVisited != 0 {
			t.Errorf("%s round %d revisited canonical history: %+v", sample.scenario, sample.round, sample.operationEstimates)
		}
		if enforceTarget {
			assertWorkListLatencyTimingTarget(t, sample.pages, sample.totalWalk)
		}
	}
}

func logWorkListStabilitySample(t *testing.T, sample workListStabilitySample) {
	t.Helper()
	pageDurations := make([]time.Duration, 0, len(sample.pages))
	for _, page := range sample.pages {
		pageDurations = append(pageDurations, page.duration)
	}
	t.Logf(
		"work list stability scenario=%s round=%d board_rows=%d worker_load=%d accumulated_dispatches=%d retained_events=%d updates_during_walk=%t pages=%d rows=%d total_walk=%s page_durations=%s event_subscription=%s operation_estimates=snapshot_reads:%d,event_subscriptions:%d,event_records_visited:%d,work_rows_projected:%d first_id=%q last_id=%q",
		sample.scenario,
		sample.round,
		sample.boardRows,
		sample.workerLoad,
		sample.accumulatedDispatches,
		sample.retainedEvents,
		sample.updatesDuringWalk,
		len(sample.pages),
		len(sample.returnedWorkIDs),
		sample.totalWalk,
		formatDurationSamples(pageDurations),
		sample.eventSubscription.duration,
		sample.operationEstimates.snapshotReads,
		sample.operationEstimates.eventSubscriptions,
		sample.operationEstimates.eventRecordsVisited,
		sample.operationEstimates.workRowsProjected,
		sample.returnedWorkIDs[0],
		sample.returnedWorkIDs[len(sample.returnedWorkIDs)-1],
	)
}

func accumulatedWorkListProfileConfig(workerName string) *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "loop", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: workerName}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "loop",
			WorkerTypeName: workerName,
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "loop"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "loop"}},
			OnFailure:      []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		}},
	}
}

func accumulatedWorkListProfileRequests(rows int, loopState string) []work.SubmitRequest {
	requests := workListProfileRequests(rows, 1)
	requests[0].TargetState = loopState
	return requests
}

type accumulatedDispatchExecutor struct {
	target      int
	checkpoints map[int]*dispatchCheckpoint
	mu          sync.Mutex
	calls       int
}

func newAccumulatedDispatchExecutor(target int) *accumulatedDispatchExecutor {
	return newCheckpointedDispatchExecutor([]int{target})
}

type dispatchCheckpoint struct {
	reached     chan struct{}
	reachedOnce sync.Once
	release     chan struct{}
	releaseOnce sync.Once
}

func newCheckpointedDispatchExecutor(checkpoints []int) *accumulatedDispatchExecutor {
	if len(checkpoints) == 0 {
		panic("checkpointed dispatch executor requires at least one checkpoint")
	}
	checkpointMap := make(map[int]*dispatchCheckpoint, len(checkpoints))
	for _, checkpoint := range checkpoints {
		checkpointMap[checkpoint] = &dispatchCheckpoint{
			reached: make(chan struct{}),
			release: make(chan struct{}),
		}
	}
	return &accumulatedDispatchExecutor{
		target:      checkpoints[len(checkpoints)-1],
		checkpoints: checkpointMap,
	}
}

func (e *accumulatedDispatchExecutor) Execute(ctx context.Context, dispatch work.WorkDispatch) (workers.WorkResult, error) {
	e.mu.Lock()
	e.calls++
	call := e.calls
	checkpoint := e.checkpoints[call]
	e.mu.Unlock()
	if checkpoint != nil {
		checkpoint.reachedOnce.Do(func() { close(checkpoint.reached) })
		select {
		case <-checkpoint.release:
		case <-ctx.Done():
			return workers.WorkResult{}, ctx.Err()
		}
	}
	if call > e.target {
		return workers.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workers.OutcomeFailed,
			Error:        "event-growth profile complete",
		}, nil
	}
	return workers.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workers.OutcomeAccepted,
	}, nil
}

func (e *accumulatedDispatchExecutor) waitForTarget(t *testing.T) {
	e.waitForCheckpoint(t, e.target)
}

func (e *accumulatedDispatchExecutor) waitForCheckpoint(t *testing.T, target int) {
	t.Helper()
	e.mu.Lock()
	checkpoint := e.checkpoints[target]
	e.mu.Unlock()
	if checkpoint == nil {
		t.Fatalf("dispatch executor has no checkpoint %d", target)
	}
	select {
	case <-checkpoint.reached:
	case <-time.After(workListProfileTimeout):
		t.Fatalf("timed out waiting for accumulated dispatch %d", target)
	}
}

func (e *accumulatedDispatchExecutor) release() {
	e.releaseCheckpoint(e.target)
}

func (e *accumulatedDispatchExecutor) releaseCheckpoint(target int) {
	e.mu.Lock()
	checkpoint := e.checkpoints[target]
	e.mu.Unlock()
	if checkpoint == nil {
		return
	}
	checkpoint.releaseOnce.Do(func() { close(checkpoint.release) })
}

func (e *accumulatedDispatchExecutor) releaseAll() {
	e.mu.Lock()
	checkpoints := make([]*dispatchCheckpoint, 0, len(e.checkpoints))
	for _, checkpoint := range e.checkpoints {
		checkpoints = append(checkpoints, checkpoint)
	}
	e.mu.Unlock()
	for _, checkpoint := range checkpoints {
		checkpoint.releaseOnce.Do(func() { close(checkpoint.release) })
	}
}

type eventSubscriptionSample struct {
	phaseSample
	retainedEvents int
}

func workListProfileRequests(rows, workerLoad int) []work.SubmitRequest {
	requests := make([]work.SubmitRequest, 0, rows)
	for index := range rows {
		request := work.SubmitRequest{
			WorkID:     fmt.Sprintf("work-list-profile-%03d", index),
			Name:       fmt.Sprintf("work-list-profile-%03d", index),
			WorkTypeID: "task",
			TraceID:    fmt.Sprintf("work-list-profile-trace-%03d", index),
			Payload:    fmt.Appendf(nil, `{"index":%d}`, index),
		}
		if index >= workerLoad {
			request.TargetState = "complete"
		}
		requests = append(requests, request)
	}
	return requests
}

func submitRequestWorkIDs(requests []work.SubmitRequest) []string {
	ids := make([]string, 0, len(requests))
	for _, request := range requests {
		ids = append(ids, request.WorkID)
	}
	return ids
}

func measureWorkListEventSubscription(
	t *testing.T,
	harness *stressProcessHarness,
) eventSubscriptionSample {
	t.Helper()
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(t.Context(), workListProfileTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, harness.baseURL+"/factory-sessions/"+factorysessions.DefaultSessionID+"/events", nil)
	if err != nil {
		t.Fatalf("build public Factory Event request: %v", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET public Factory Events: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		t.Fatalf("GET public Factory Events status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	retainedText := response.Header.Get(recordingshttp.SessionEventStreamRetainedCountHeader)
	retainedEvents, err := strconv.Atoi(retainedText)
	if err != nil || retainedEvents < 0 {
		t.Fatalf("public Factory Event retained count = %q, want non-negative integer", retainedText)
	}
	bytes := response.ContentLength
	if bytes < 0 {
		bytes = 0
	}
	cancel()
	return eventSubscriptionSample{
		phaseSample:    phaseSample{duration: time.Since(startedAt), bytes: int(bytes)},
		retainedEvents: retainedEvents,
	}
}

func measureWorkListSessionRead(t *testing.T, harness *stressProcessHarness) phaseSample {
	t.Helper()
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(t.Context(), workListProfileTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, harness.sessionURL(), nil)
	if err != nil {
		t.Fatalf("build public Factory Session request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET public Factory Session: %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatalf("read public Factory Session response: %v", readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET public Factory Session status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return phaseSample{duration: time.Since(startedAt), bytes: len(body)}
}

func walkWorkListPages(
	t *testing.T,
	harness *stressProcessHarness,
	expectedRequests []work.SubmitRequest,
	workerLoad int,
) ([]workListProfilePage, time.Duration, []string) {
	return walkWorkListPagesWithHook(t, harness, expectedRequests, workerLoad, nil)
}

func walkWorkListPagesWithHook(
	t *testing.T,
	harness *stressProcessHarness,
	expectedRequests []work.SubmitRequest,
	workerLoad int,
	beforePage func(int),
) ([]workListProfilePage, time.Duration, []string) {
	t.Helper()
	expected := make(map[string]struct{}, len(expectedRequests))
	for _, request := range expectedRequests {
		expected[request.WorkID] = struct{}{}
	}
	expectedOrder := expectedWorkListOrder(expectedRequests, workerLoad)
	seen := make(map[string]struct{}, len(expected))
	pages := make([]workListProfilePage, 0, (len(expected)+workListProfilePageSize-1)/workListProfilePageSize)
	returned := make([]string, 0, len(expected))
	var nextToken string
	startedWalk := time.Now()
	for pageNumber := 1; ; pageNumber++ {
		if beforePage != nil {
			beforePage(pageNumber)
		}
		query := url.Values{}
		query.Set("maxResults", strconv.Itoa(workListProfilePageSize))
		if nextToken != "" {
			query.Set("nextToken", nextToken)
		}
		page := requestWorkListPage(t, harness, pageNumber, query.Encode())
		expectedPageRows := workListProfilePageSize
		remainingRows := len(expected) - (pageNumber-1)*workListProfilePageSize
		if remainingRows < expectedPageRows {
			expectedPageRows = remainingRows
		}
		if len(page.response.Results) != expectedPageRows {
			t.Fatalf("page %d returned %d rows, want %d", pageNumber, len(page.response.Results), expectedPageRows)
		}
		pageIDs := make([]string, 0, len(page.response.Results))
		for _, item := range page.response.Results {
			if item.WorkId == nil || strings.TrimSpace(*item.WorkId) == "" {
				t.Fatalf("page %d returned Work without concrete workId: %#v", pageNumber, item)
			}
			workID := *item.WorkId
			if _, ok := expected[workID]; !ok {
				t.Fatalf("page %d returned unexpected Work ID %q", pageNumber, workID)
			}
			if _, duplicate := seen[workID]; duplicate {
				t.Fatalf("page %d returned duplicate Work ID %q", pageNumber, workID)
			}
			seen[workID] = struct{}{}
			pageIDs = append(pageIDs, workID)
			returned = append(returned, workID)
		}
		page.nextToken = responseNextToken(page.response)
		expectedPageCount := (len(expected) + workListProfilePageSize - 1) / workListProfilePageSize
		if pageNumber < expectedPageCount && page.nextToken == "" {
			t.Fatalf("page %d ended pagination early", pageNumber)
		}
		if pageNumber == expectedPageCount && page.nextToken != "" {
			t.Fatalf("page %d returned an unexpected nextToken", pageNumber)
		}
		page.resultIDs = pageIDs
		pages = append(pages, page.workListProfilePage)
		if page.nextToken == "" {
			break
		}
		if page.nextToken == nextToken {
			t.Fatalf("page %d repeated nextToken %q", pageNumber, nextToken)
		}
		nextToken = page.nextToken
		if pageNumber > len(pages)+1 || pageNumber > len(expected)/workListProfilePageSize+2 {
			t.Fatalf("pagination did not converge after page %d", pageNumber)
		}
	}
	if len(seen) != len(expected) {
		t.Fatalf("paginated Work census returned %d unique IDs, want %d; IDs=%v", len(seen), len(expected), returned)
	}
	if len(returned) != len(expectedOrder) {
		t.Fatalf("paginated Work order has %d IDs, want %d", len(returned), len(expectedOrder))
	}
	for index, workID := range expectedOrder {
		if returned[index] != workID {
			t.Fatalf("paginated Work order[%d] = %q, want %q; returned=%v", index, returned[index], workID, returned)
		}
	}
	return pages, time.Since(startedWalk), returned
}

func expectedWorkListOrder(requests []work.SubmitRequest, workerLoad int) []string {
	indexes := make([]int, len(requests))
	for index := range indexes {
		indexes[index] = index
	}
	sort.Slice(indexes, func(left, right int) bool {
		leftRank := workListStateRank(indexes[left], workerLoad)
		rightRank := workListStateRank(indexes[right], workerLoad)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return profileCursorID(indexes[left]) < profileCursorID(indexes[right])
	})
	ordered := make([]string, 0, len(requests))
	for _, index := range indexes {
		ordered = append(ordered, requests[index].WorkID)
	}
	return ordered
}

func workListStateRank(index, workerLoad int) int {
	if index < workerLoad {
		return 1
	}
	return 3
}

func profileCursorID(index int) string {
	return fmt.Sprintf("tok-task-%d", index+1)
}

type fetchedWorkListPage struct {
	workListProfilePage
	response factoryapi.ListWorkResponse
}

func requestWorkListPage(
	t *testing.T,
	harness *stressProcessHarness,
	pageNumber int,
	encodedQuery string,
) fetchedWorkListPage {
	t.Helper()
	endpoint := harness.baseURL + "/factory-sessions/" + factorysessions.DefaultSessionID + "/work?" + encodedQuery
	ctx, cancel := context.WithTimeout(t.Context(), workListProfileTimeout)
	defer cancel()
	startedAt := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build Work list page %d request: %v", pageNumber, err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET Work list page %d: %v", pageNumber, err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatalf("read Work list page %d: %v", pageNumber, readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET Work list page %d status = %d: %s", pageNumber, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded factoryapi.ListWorkResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode Work list page %d: %v", pageNumber, err)
	}
	if decoded.PaginationContext == nil || decoded.PaginationContext.MaxResults != workListProfilePageSize {
		t.Fatalf("Work list page %d pagination = %#v, want maxResults=%d", pageNumber, decoded.PaginationContext, workListProfilePageSize)
	}
	if len(decoded.Results) > workListProfilePageSize {
		t.Fatalf("Work list page %d returned %d rows, want at most %d", pageNumber, len(decoded.Results), workListProfilePageSize)
	}
	return fetchedWorkListPage{
		workListProfilePage: workListProfilePage{
			number:   pageNumber,
			duration: time.Since(startedAt),
			bytes:    len(body),
		},
		response: decoded,
	}
}

func responseNextToken(response factoryapi.ListWorkResponse) string {
	if response.PaginationContext == nil || response.PaginationContext.NextToken == nil {
		return ""
	}
	return *response.PaginationContext.NextToken
}

func logWorkListLatencyProfile(t *testing.T, profile workListLatencyProfile) {
	t.Helper()
	pageDurations := make([]time.Duration, 0, len(profile.pages))
	pageBytes := make([]int, 0, len(profile.pages))
	for _, page := range profile.pages {
		pageDurations = append(pageDurations, page.duration)
		pageBytes = append(pageBytes, page.bytes)
	}
	t.Logf(
		"work list profile scenario=%s board_rows=%d worker_load=%d accumulated_dispatches=%d retained_events=%d pages=%d rows=%d total_walk=%s page_durations=%s page_bytes=%v snapshot_read=%s/%dB event_subscription=%s/%dB operation_estimates=snapshot_reads:%d,event_subscriptions:%d,event_records_visited:%d,work_rows_projected:%d first_id=%q last_id=%q",
		profile.scenario,
		profile.boardRows,
		profile.workerLoad,
		profile.accumulatedDispatches,
		profile.retainedEvents,
		len(profile.pages),
		len(profile.returnedWorkIDs),
		profile.totalWalk,
		formatDurationSamples(pageDurations),
		pageBytes,
		profile.snapshotRead.duration,
		profile.snapshotRead.bytes,
		profile.eventSubscription.duration,
		profile.eventSubscription.bytes,
		profile.operationEstimates.snapshotReads,
		profile.operationEstimates.eventSubscriptions,
		profile.operationEstimates.eventRecordsVisited,
		profile.operationEstimates.workRowsProjected,
		profile.returnedWorkIDs[0],
		profile.returnedWorkIDs[len(profile.returnedWorkIDs)-1],
	)
	if len(profile.returnedWorkIDs) != len(profile.expectedWorkIDs) {
		t.Fatalf("profile Work ID census = %d, want %d", len(profile.returnedWorkIDs), len(profile.expectedWorkIDs))
	}
}

func assertWorkListLatencyTarget(t *testing.T, profile workListLatencyProfile) {
	t.Helper()
	assertWorkListLatencyTimingTarget(t, profile.pages, profile.totalWalk)
}

func assertWorkListLatencyTimingTarget(t *testing.T, pages []workListProfilePage, totalWalk time.Duration) {
	t.Helper()
	for _, page := range pages {
		if page.duration >= workListProfilePageLimit {
			t.Errorf("Work list profile page %d = %s, want < %s", page.number, page.duration, workListProfilePageLimit)
		}
	}
	if totalWalk >= workListProfileWalkLimit {
		t.Errorf("Work list profile walk = %s, want < %s", totalWalk, workListProfileWalkLimit)
	}
}

var _ workers.WorkerExecutor = (*queryLatencyExecutor)(nil)
var _ workers.WorkerExecutor = (*accumulatedDispatchExecutor)(nil)

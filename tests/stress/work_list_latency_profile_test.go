package stress_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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
	started     chan struct{}
	releaseCh   chan struct{}
	releaseOnce sync.Once
	mu          sync.Mutex
	calls       int
}

func newAccumulatedDispatchExecutor(target int) *accumulatedDispatchExecutor {
	return &accumulatedDispatchExecutor{
		target:    target,
		started:   make(chan struct{}),
		releaseCh: make(chan struct{}),
	}
}

func (e *accumulatedDispatchExecutor) Execute(ctx context.Context, dispatch work.WorkDispatch) (workers.WorkResult, error) {
	e.mu.Lock()
	e.calls++
	call := e.calls
	e.mu.Unlock()
	if call == e.target {
		close(e.started)
		select {
		case <-e.releaseCh:
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
	t.Helper()
	select {
	case <-e.started:
	case <-time.After(workListProfileTimeout):
		t.Fatalf("timed out waiting for accumulated dispatch %d", e.target)
	}
}

func (e *accumulatedDispatchExecutor) release() {
	e.releaseOnce.Do(func() { close(e.releaseCh) })
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
	for _, page := range profile.pages {
		if page.duration >= workListProfilePageLimit {
			t.Errorf("Work list profile page %d = %s, want < %s", page.number, page.duration, workListProfilePageLimit)
		}
	}
	if profile.totalWalk >= workListProfileWalkLimit {
		t.Errorf("Work list profile walk = %s, want < %s", profile.totalWalk, workListProfileWalkLimit)
	}
}

var _ workers.WorkerExecutor = (*queryLatencyExecutor)(nil)
var _ workers.WorkerExecutor = (*accumulatedDispatchExecutor)(nil)

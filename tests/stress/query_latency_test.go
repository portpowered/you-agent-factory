package stress_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	queryLatencyProbeCount = 14
	queryLatencyTimeout    = 15 * time.Second
	queryLatencyWait       = 10 * time.Second
)

// TestWorkAndWorkerSessionQueryLatencyMatrix records the unoptimized HTTP
// read-path baseline across independent board-size and worker-load axes. The
// test intentionally reports hard failures instead of asserting a latency
// target; the follow-up responsiveness story turns this characterization into
// a regression test after the owner of the measured bottleneck is corrected.
func TestWorkAndWorkerSessionQueryLatencyMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping query latency matrix in short mode")
	}

	for _, boardSize := range []int{10, 52} {
		workerLevels := []int{0, minInt(9, boardSize), minInt(27, boardSize)}
		for _, workerLoad := range workerLevels {
			name := fmt.Sprintf("board-%d-workers-%d", boardSize, workerLoad)
			t.Run(name, func(t *testing.T) {
				runQueryLatencyCell(t, boardSize, workerLoad)
			})
		}
	}
}

func runQueryLatencyCell(t *testing.T, boardSize, workerLoad int) {
	t.Helper()

	const workerName = "query-latency-worker"
	dir := testutil.ScaffoldFactoryDir(t, testutil.PipelineConfig(1, workerName))
	harness := startStressProcessWithWorkerMux(t, dir)
	executor := newQueryLatencyExecutor(boardSize)
	harness.SetWorkerExecutor(workerName, executor)

	requests := queryLatencyRequests(boardSize, workerLoad)
	submittedAt := time.Now()
	harness.SubmitFull(t.Context(), requests)
	acceptedAt := time.Now()

	var dispatchStarts []time.Time
	if workerLoad > 0 {
		dispatchStarts = executor.waitForStarts(t, workerLoad)
	}

	workStats, workerSessionStats := runConcurrentQueryProbes(t, harness, queryLatencyProbeCount)
	dispatchStats := summarizeDispatchStartTimes(dispatchStarts, acceptedAt)
	cliStats := queryStats{}
	if boardSize == 52 && workerLoad == 27 {
		cliStats = runWorkListCLIRoundTrips(t, harness, queryLatencyProbeCount)
	}

	t.Logf(
		"query latency matrix cell board_rows=%d worker_load=%d probes=%d %s %s %s cli_work_round_trip=%s submit_to_accept=%s",
		boardSize,
		workerLoad,
		queryLatencyProbeCount,
		workStats,
		workerSessionStats,
		dispatchStats,
		cliStats,
		acceptedAt.Sub(submittedAt),
	)

	executor.release()
	harness.WaitForTerminalCount(boardSize, queryLatencyWait)
	harness.Stop()
}

func runWorkListCLIRoundTrips(
	t *testing.T,
	harness *stressProcessHarness,
	probeCount int,
) queryStats {
	t.Helper()
	probes := make([]queryProbe, 0, probeCount)
	workingDirectory := t.TempDir()
	for range probeCount {
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(t.Context(), queryLatencyTimeout)
		count := 0
		process, err := root.BuildProcess(ctx, serviceedges.Edges{})
		if err == nil {
			var output bytes.Buffer
			errOutput := io.Discard
			err = process.Execute(root.Input{
				Args:             []string{"you", "--server", harness.baseURL, "--json", "work", "list", "--max-results", "200"},
				Env:              os.Environ(),
				Stdout:           &output,
				Stderr:           errOutput,
				Context:          ctx,
				WorkingDirectory: workingDirectory,
			})
			if err == nil {
				var response factoryapi.ListWorkResponse
				err = json.Unmarshal(output.Bytes(), &response)
				count = len(response.Results)
				if err == nil && count != 52 {
					err = fmt.Errorf("CLI returned %d Work rows, want 52", count)
				}
			}
			if closeErr := process.Close(context.Background()); err == nil {
				err = closeErr
			}
		}
		cancel()
		probes = append(probes, queryProbe{latency: time.Since(startedAt), count: count, err: err})
	}
	return summarizeQueryProbes(probes)
}

func queryLatencyRequests(boardSize, workerLoad int) []work.SubmitRequest {
	requests := make([]work.SubmitRequest, 0, boardSize)
	for index := range boardSize {
		request := work.SubmitRequest{
			WorkID:     fmt.Sprintf("query-latency-work-%03d", index),
			Name:       fmt.Sprintf("query-latency-work-%03d", index),
			WorkTypeID: "task",
			TraceID:    fmt.Sprintf("query-latency-trace-%03d", index),
			Payload:    fmt.Appendf(nil, `{"index":%d}`, index),
		}
		if index >= workerLoad {
			request.TargetState = "complete"
		}
		requests = append(requests, request)
	}
	return requests
}

type queryProbe struct {
	latency time.Duration
	count   int
	err     error
}

type queryStats struct {
	probes       int
	successes    int
	hardFailures int
	counts       map[int]int
	median       time.Duration
	max          time.Duration
	firstError   string
}

func (s queryStats) String() string {
	return fmt.Sprintf(
		"success=%d hard_failures=%d rows=%s p50=%s max=%s first_error=%q",
		s.successes,
		s.hardFailures,
		formatCountDistribution(s.counts),
		s.median,
		s.max,
		s.firstError,
	)
}

func runConcurrentQueryProbes(
	t *testing.T,
	harness *stressProcessHarness,
	probeCount int,
) (queryStats, queryStats) {
	t.Helper()

	workEndpoint := fmt.Sprintf(
		"%s/factory-sessions/%s/work?maxResults=200",
		strings.TrimSuffix(harness.baseURL, "/"),
		factorysessions.DefaultSessionID,
	)
	workerSessionEndpoint := fmt.Sprintf(
		"%s/worker-sessions?scope=factory&maxResults=200",
		strings.TrimSuffix(harness.baseURL, "/"),
	)

	workResults := make(chan queryProbe, probeCount)
	workerSessionResults := make(chan queryProbe, probeCount)
	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	for range probeCount {
		waitGroup.Add(2)
		go func() {
			defer waitGroup.Done()
			<-start
			workResults <- probeListEndpoint(t.Context(), workEndpoint, decodeWorkList)
		}()
		go func() {
			defer waitGroup.Done()
			<-start
			workerSessionResults <- probeListEndpoint(t.Context(), workerSessionEndpoint, decodeWorkerSessionList)
		}()
	}
	close(start)
	waitGroup.Wait()
	close(workResults)
	close(workerSessionResults)

	workProbes := make([]queryProbe, 0, probeCount)
	for probe := range workResults {
		workProbes = append(workProbes, probe)
	}
	workerSessionProbes := make([]queryProbe, 0, probeCount)
	for probe := range workerSessionResults {
		workerSessionProbes = append(workerSessionProbes, probe)
	}
	return summarizeQueryProbes(workProbes), summarizeQueryProbes(workerSessionProbes)
}

func probeListEndpoint(
	parent context.Context,
	endpoint string,
	decode func(*json.Decoder) (int, error),
) queryProbe {
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(parent, queryLatencyTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return queryProbe{latency: time.Since(startedAt), err: err}
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return queryProbe{latency: time.Since(startedAt), err: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return queryProbe{
			latency: time.Since(startedAt),
			err: fmt.Errorf(
				"status=%d body=%q",
				response.StatusCode,
				strings.TrimSpace(string(body)),
			),
		}
	}
	count, err := decode(json.NewDecoder(response.Body))
	return queryProbe{latency: time.Since(startedAt), count: count, err: err}
}

func decodeWorkList(decoder *json.Decoder) (int, error) {
	var response factoryapi.ListWorkResponse
	if err := decoder.Decode(&response); err != nil {
		return 0, err
	}
	return len(response.Results), nil
}

func decodeWorkerSessionList(decoder *json.Decoder) (int, error) {
	var response factoryapi.ListWorkerSessionsResponse
	if err := decoder.Decode(&response); err != nil {
		return 0, err
	}
	return len(response.Sessions), nil
}

func summarizeQueryProbes(probes []queryProbe) queryStats {
	stats := queryStats{probes: len(probes), counts: make(map[int]int)}
	durations := make([]time.Duration, 0, len(probes))
	for _, probe := range probes {
		if probe.err != nil {
			stats.hardFailures++
			if stats.firstError == "" {
				stats.firstError = probe.err.Error()
			}
			continue
		}
		stats.successes++
		stats.counts[probe.count]++
		durations = append(durations, probe.latency)
		if probe.latency > stats.max {
			stats.max = probe.latency
		}
	}
	if len(durations) == 0 {
		return stats
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	stats.median = durations[(len(durations)-1)/2]
	return stats
}

func formatCountDistribution(counts map[int]int) string {
	if len(counts) == 0 {
		return "{}"
	}
	values := make([]int, 0, len(counts))
	for value := range counts {
		values = append(values, value)
	}
	sort.Ints(values)
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%d:%d", value, counts[value]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

type dispatchStartStats struct {
	count  int
	median time.Duration
	max    time.Duration
}

func (s dispatchStartStats) String() string {
	return fmt.Sprintf("dispatch_start=%d p50=%s max=%s", s.count, s.median, s.max)
}

func summarizeDispatchStartTimes(starts []time.Time, admittedAt time.Time) dispatchStartStats {
	stats := dispatchStartStats{count: len(starts)}
	if len(starts) == 0 {
		return stats
	}
	durations := make([]time.Duration, 0, len(starts))
	for _, startedAt := range starts {
		duration := startedAt.Sub(admittedAt)
		if duration < 0 {
			duration = 0
		}
		durations = append(durations, duration)
		if duration > stats.max {
			stats.max = duration
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	stats.median = durations[(len(durations)-1)/2]
	return stats
}

type queryLatencyExecutor struct {
	started     chan time.Time
	releaseCh   chan struct{}
	releaseOnce sync.Once
}

func newQueryLatencyExecutor(boardSize int) *queryLatencyExecutor {
	return &queryLatencyExecutor{
		started:   make(chan time.Time, boardSize*4+1),
		releaseCh: make(chan struct{}),
	}
}

func (e *queryLatencyExecutor) Execute(ctx context.Context, dispatch work.WorkDispatch) (workers.WorkResult, error) {
	e.started <- time.Now()
	select {
	case <-e.releaseCh:
	case <-ctx.Done():
		return workers.WorkResult{}, ctx.Err()
	}
	return workers.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workers.OutcomeAccepted,
	}, nil
}

func (e *queryLatencyExecutor) waitForStarts(t *testing.T, count int) []time.Time {
	t.Helper()
	starts := make([]time.Time, 0, count)
	deadline := time.NewTimer(queryLatencyWait)
	defer deadline.Stop()
	for range count {
		select {
		case startedAt := <-e.started:
			starts = append(starts, startedAt)
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d dispatched workers; received %d", count, len(starts))
		}
	}
	return starts
}

func (e *queryLatencyExecutor) release() {
	e.releaseOnce.Do(func() { close(e.releaseCh) })
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

var _ workers.WorkerExecutor = (*queryLatencyExecutor)(nil)

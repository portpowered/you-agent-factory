package workscope_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	prebuiltWorkscopeBinaryEnv       = "INFINITE_YOU_INTEGRATION_BINARY"
	prebuiltWorkscopeFallbackEnv     = "INFINITE_YOU_PREBUILT_ARTIFACT"
	prebuiltWorkscopeRequiredEnv     = "INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT"
	prebuiltWorkscopeFactoryID       = "~default"
	prebuiltWorkscopeWorkID          = "cost-replay-priced-work"
	prebuiltWorkscopeWorkerSession   = "cost-replay-priced-worker-session"
	prebuiltWorkscopeProviderSession = "cost-replay-priced-provider-session"
	prebuiltWorkscopeBudget          = 2 * time.Second
	prebuiltWorkscopeTimeout         = 10 * time.Minute
)

// TestPrebuiltWorkscopeCopiedRestartPerformance exercises the customer HTTP
// and CLI surfaces through one SHA-256-pinned binary. It copies a pinned
// retained-history ledger, measures its public journey, then copies that ledger
// and measures it again in a fresh process. The test never builds the CLI or
// invokes a real provider.
type prebuiltWorkscopeScenario struct {
	root                     string
	workspace                string
	profile                  string
	serverURL                string
	initialRecording         string
	binaryPath               string
	binarySHA                string
	binaryBytes              int64
	sourceFixturePath        string
	sourceFixtureSHA         string
	sourceFixtureBytes       int64
	replayFixture            string
	replayFixtureSHA         string
	replayFixtureBytes       int64
	providerTranscriptSource string
	providerTranscriptSHA    string
	providerTranscriptBytes  int64
	firstEnvironment         []string
}

type prebuiltWorkscopePhase struct {
	journey                  prebuiltWorkscopeJourney
	resolvedFactorySessionID string
	highWater                uint64
	memoryMethod             string
}

func TestPrebuiltWorkscopeCopiedRestartPerformance(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), prebuiltWorkscopeTimeout)
	defer cancel()
	scenario := preparePrebuiltWorkscopeScenario(t)
	before := runPrebuiltWorkscopeInitialPhase(t, ctx, scenario)
	after := runPrebuiltWorkscopeCopiedPhase(t, ctx, scenario, before)
	assertPrebuiltWorkscopeCopiedRestart(t, scenario, before, after)
	logPrebuiltWorkscopeReport(t, scenario, before, after)
}

func preparePrebuiltWorkscopeScenario(t *testing.T) prebuiltWorkscopeScenario {
	binaryPath := resolvePrebuiltWorkscopeBinary(t)
	binarySHA, binaryBytes := sha256File(t, binaryPath)
	fixturePath := testutil.MustRepoPath(t, "tests/functional/factory/visualization/runtime_metrics/testdata/codex-gpt-5-codex.factory-recording.v1.json")
	fixtureSHA, fixtureBytes := sha256File(t, fixturePath)
	providerTranscriptSource := testutil.MustRepoPath(t, "tests/functional/internal/support/testdata/provider-sessions/codex/success/rollout.jsonl")
	providerTranscriptSHA, providerTranscriptBytes := sha256File(t, providerTranscriptSource)
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	profile := filepath.Join(root, "profile")
	inputDir := filepath.Join(root, "input")
	for _, directory := range []string{workspace, profile, inputDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create isolated prebuilt Work Session directory %s: %v", directory, err)
		}
	}
	replayFixture := filepath.Join(inputDir, "workscope-retained-history.json")
	preparePrebuiltWorkscopeFixture(t, fixturePath, replayFixture)
	replayFixtureSHA, replayFixtureBytes := sha256File(t, replayFixture)
	providerPath := filepath.Join(profile, ".codex", "sessions", "rollout-"+prebuiltWorkscopeProviderSession+".jsonl")
	if err := os.MkdirAll(filepath.Dir(providerPath), 0o700); err != nil {
		t.Fatalf("create isolated prebuilt Provider Session store: %v", err)
	}
	copyWorkscopeFile(t, providerTranscriptSource, providerPath)
	if copiedSHA, _ := sha256File(t, providerPath); copiedSHA != providerTranscriptSHA {
		t.Fatalf("isolated Provider Session transcript SHA-256 = %s, want fixture SHA-256 %s", copiedSHA, providerTranscriptSHA)
	}
	port, err := builtcliacceptance.ReserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve isolated Work Session listener: %v", err)
	}
	if port == 7437 {
		t.Fatalf("reserved Work Session listener uses forbidden default port %d", port)
	}
	return prebuiltWorkscopeScenario{
		root: root, workspace: workspace, profile: profile,
		serverURL:        fmt.Sprintf("http://127.0.0.1:%d", port),
		initialRecording: filepath.Join(root, "source.recording.json"),
		binaryPath:       binaryPath, binarySHA: binarySHA, binaryBytes: binaryBytes,
		sourceFixturePath: fixturePath, sourceFixtureSHA: fixtureSHA, sourceFixtureBytes: fixtureBytes,
		replayFixture: replayFixture, replayFixtureSHA: replayFixtureSHA, replayFixtureBytes: replayFixtureBytes,
		providerTranscriptSource: providerTranscriptSource,
		providerTranscriptSHA:    providerTranscriptSHA, providerTranscriptBytes: providerTranscriptBytes,
		firstEnvironment: builtcliacceptance.ProcessEnvForIsolatedHome(profile),
	}
}

func runPrebuiltWorkscopeInitialPhase(t *testing.T, ctx context.Context, scenario prebuiltWorkscopeScenario) prebuiltWorkscopePhase {
	copyWorkscopeFile(t, scenario.replayFixture, scenario.initialRecording)
	daemon := startPrebuiltWorkscopeDaemon(t, ctx, scenario.binaryPath, scenario.workspace, scenario.firstEnvironment,
		"run", "--dir", scenario.workspace,
		"--replay", scenario.initialRecording,
		"--with-server", "--server", scenario.serverURL,
		"--continuously", "--quiet",
	)
	waitForPrebuiltWorkscopeTerminal(t, ctx, daemon, scenario.serverURL, prebuiltWorkscopeFactoryID)
	journey := measurePrebuiltWorkscopeJourney(t, ctx, prebuiltWorkscopeFactoryID, scenario.serverURL)
	resolvedID, commands := assertPrebuiltWorkscopeCLIParity(t, ctx, scenario.binaryPath, scenario.workspace, scenario.firstEnvironment, scenario.serverURL, journey)
	journey.sourceCalls.PublicCLICommands = commands
	normalizePrebuiltWorkscopeFactorySession(&journey, resolvedID)
	highWater, memoryMethod, err := daemon.stopMemoryMonitor()
	if err != nil {
		t.Fatalf("measure source prebuilt process working-set high-water: %v", err)
	}
	stopPrebuiltWorkscopeDaemon(t, ctx, daemon, scenario.binaryPath, scenario.workspace, scenario.firstEnvironment, scenario.serverURL)
	return prebuiltWorkscopePhase{journey: journey, resolvedFactorySessionID: resolvedID, highWater: highWater, memoryMethod: memoryMethod}
}

func runPrebuiltWorkscopeCopiedPhase(t *testing.T, ctx context.Context, scenario prebuiltWorkscopeScenario, before prebuiltWorkscopePhase) prebuiltWorkscopePhase {
	copiedRecording := filepath.Join(scenario.root, "copied.recording.json")
	copyWorkscopeFile(t, scenario.initialRecording, copiedRecording)
	if copiedInfo, err := os.Stat(copiedRecording); err != nil || copiedInfo.Size() == 0 {
		t.Fatalf("copied retained-history recording size: info=%v err=%v", copiedInfo, err)
	}
	secondProfile := filepath.Join(scenario.root, "restart-profile")
	copyPrebuiltWorkscopeDirectory(t, scenario.profile, secondProfile)
	providerPath := filepath.Join(secondProfile, ".codex", "sessions", "rollout-"+prebuiltWorkscopeProviderSession+".jsonl")
	if copiedSHA, _ := sha256File(t, providerPath); copiedSHA != scenario.providerTranscriptSHA {
		t.Fatalf("copied Provider Session transcript SHA-256 = %s, want fixture SHA-256 %s", copiedSHA, scenario.providerTranscriptSHA)
	}
	environment := builtcliacceptance.ProcessEnvForIsolatedHome(secondProfile)
	daemon := startPrebuiltWorkscopeDaemon(t, ctx, scenario.binaryPath, scenario.workspace, environment,
		"run", "--dir", scenario.workspace,
		"--session", before.resolvedFactorySessionID,
		"--replay", copiedRecording,
		"--with-server", "--server", scenario.serverURL,
		"--continuously", "--quiet",
	)
	waitForPrebuiltWorkscopeTerminal(t, ctx, daemon, scenario.serverURL, before.resolvedFactorySessionID)
	journey := measurePrebuiltWorkscopeJourney(t, ctx, before.resolvedFactorySessionID, scenario.serverURL)
	resolvedID, commands := assertPrebuiltWorkscopeCLIParity(t, ctx, scenario.binaryPath, scenario.workspace, environment, scenario.serverURL, journey)
	journey.sourceCalls.PublicCLICommands = commands
	if resolvedID != before.resolvedFactorySessionID {
		t.Fatalf("copied Factory Session identity changed across restart: before=%q after=%q", before.resolvedFactorySessionID, resolvedID)
	}
	normalizePrebuiltWorkscopeFactorySession(&journey, resolvedID)
	highWater, memoryMethod, err := daemon.stopMemoryMonitor()
	if err != nil {
		t.Fatalf("measure resumed prebuilt process working-set high-water: %v", err)
	}
	stopPrebuiltWorkscopeDaemon(t, ctx, daemon, scenario.binaryPath, scenario.workspace, environment, scenario.serverURL)
	return prebuiltWorkscopePhase{journey: journey, resolvedFactorySessionID: resolvedID, highWater: highWater, memoryMethod: memoryMethod}
}

func assertPrebuiltWorkscopeCopiedRestart(t *testing.T, scenario prebuiltWorkscopeScenario, before, after prebuiltWorkscopePhase) {
	t.Helper()
	assertPrebuiltWorkscopeRestartEqual(t, before.journey, after.journey)
	assertPrebuiltWorkscopeFixtureUnchanged(t, scenario)
	if before.memoryMethod != after.memoryMethod {
		t.Fatalf("process memory measurement method changed across restart: %q -> %q", before.memoryMethod, after.memoryMethod)
	}
	if highWater := maxUint64(before.highWater, after.highWater); highWater > 2<<30 {
		t.Fatalf("prebuilt process working-set high-water = %d bytes, above the 2 GiB witness budget", highWater)
	}
}

func assertPrebuiltWorkscopeFixtureUnchanged(t *testing.T, scenario prebuiltWorkscopeScenario) {
	checks := []struct{ path, want, name string }{
		{scenario.binaryPath, scenario.binarySHA, "prebuilt artifact"},
		{scenario.sourceFixturePath, scenario.sourceFixtureSHA, "source fixture"},
		{scenario.replayFixture, scenario.replayFixtureSHA, "prepared fixture"},
		{scenario.providerTranscriptSource, scenario.providerTranscriptSHA, "source Provider Session fixture"},
	}
	for _, check := range checks {
		if got := sha256FileDigest(t, check.path); got != check.want {
			t.Fatalf("%s changed during witness: before=%s after=%s", check.name, check.want, got)
		}
	}
}

func logPrebuiltWorkscopeReport(t *testing.T, scenario prebuiltWorkscopeScenario, before, after prebuiltWorkscopePhase) {
	report := prebuiltWorkscopePerformanceReport{
		ArtifactSHA256: scenario.binarySHA, ArtifactBytes: scenario.binaryBytes,
		SourceFixtureSHA256: scenario.sourceFixtureSHA, SourceFixtureBytes: scenario.sourceFixtureBytes,
		FixtureSHA256: scenario.replayFixtureSHA, FixtureBytes: scenario.replayFixtureBytes,
		ProviderTranscriptSHA256: scenario.providerTranscriptSHA, ProviderTranscriptBytes: scenario.providerTranscriptBytes,
		ProviderSessionID: prebuiltWorkscopeProviderSession, FactorySession: prebuiltWorkscopeFactoryID,
		ResolvedFactorySessionIDs: prebuiltWorkscopeResolvedFactorySessionIDs{
			Before: before.resolvedFactorySessionID, After: after.resolvedFactorySessionID,
		},
		WorkID: prebuiltWorkscopeWorkID, WorkerSession: prebuiltWorkscopeWorkerSession,
		Before: before.journey.measurements, After: after.journey.measurements,
		SourceCalls: prebuiltWorkscopeSourceCalls{Before: before.journey.sourceCalls, After: after.journey.sourceCalls},
		Cancellation: prebuiltWorkscopeCancellation{
			BeforeRequestCanceled: before.journey.cancellationRequested, BeforeBodyClosed: before.journey.cancellationBodyClosed,
			AfterRequestCanceled: after.journey.cancellationRequested, AfterBodyClosed: after.journey.cancellationBodyClosed,
		},
		Process: prebuiltWorkscopeProcessMeasurement{
			MemoryMethod: before.memoryMethod, BeforeWorkingSetHighWater: before.highWater,
			AfterWorkingSetHighWater: after.highWater, WorkingSetHighWaterBytes: maxUint64(before.highWater, after.highWater),
			MaximumConcurrentChildren: 2,
		},
	}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("encode prebuilt Work Session performance report: %v", err)
	}
	t.Logf("PERF-WS-2S report=%s", reportBytes)
}

type prebuiltWorkscopeDaemon struct {
	command *exec.Cmd
	stdout  *prebuiltWorkscopeBuffer
	stderr  *prebuiltWorkscopeBuffer
	done    chan error
	memory  *prebuiltWorkscopeMemoryMonitor
	stopped bool
	mu      sync.Mutex
}

type prebuiltWorkscopeMemoryMonitor struct {
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
	high     uint64
	method   string
	lastErr  error
}

func startPrebuiltWorkscopeMemoryMonitor(pid int) *prebuiltWorkscopeMemoryMonitor {
	monitor := &prebuiltWorkscopeMemoryMonitor{stop: make(chan struct{}), done: make(chan struct{})}
	go monitor.run(pid)
	return monitor
}

func (monitor *prebuiltWorkscopeMemoryMonitor) run(pid int) {
	defer close(monitor.done)
	monitor.sample(pid)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-monitor.stop:
			return
		case <-ticker.C:
			monitor.sample(pid)
		}
	}
}

func (monitor *prebuiltWorkscopeMemoryMonitor) sample(pid int) {
	workingSet, method, err := prebuiltProcessWorkingSetHighWater(pid)
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	if err != nil {
		monitor.lastErr = err
		return
	}
	if workingSet > monitor.high {
		monitor.high = workingSet
	}
	monitor.method = method
	monitor.lastErr = nil
}

func (monitor *prebuiltWorkscopeMemoryMonitor) stopAndRead() (uint64, string, error) {
	if monitor == nil {
		return 0, "", errors.New("prebuilt process memory monitor is unavailable")
	}
	monitor.stopOnce.Do(func() { close(monitor.stop) })
	<-monitor.done
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	if monitor.high == 0 {
		if monitor.lastErr != nil {
			return 0, monitor.method, monitor.lastErr
		}
		return 0, monitor.method, errors.New("prebuilt process memory monitor recorded no samples")
	}
	return monitor.high, monitor.method, nil
}

func (daemon *prebuiltWorkscopeDaemon) stopMemoryMonitor() (uint64, string, error) {
	if daemon == nil || daemon.memory == nil {
		return 0, "", errors.New("prebuilt process memory monitor is unavailable")
	}
	return daemon.memory.stopAndRead()
}

type prebuiltWorkscopeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (buffer *prebuiltWorkscopeBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buf.Write(value)
}

func (buffer *prebuiltWorkscopeBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buf.String()
}

type prebuiltWorkscopeMeasurements struct {
	ListHeadersMillis     float64 `json:"list_headers_ms"`
	CompleteListMillis    float64 `json:"complete_list_ms"`
	ObservationReadMillis float64 `json:"observation_read_ms"`
	StableIDReadMillis    float64 `json:"stable_id_read_ms"`
	FirstRetainedEventMS  float64 `json:"first_retained_event_ms"`
	Rows                  int     `json:"rows"`
	Events                int     `json:"events"`
	ListBodyBytes         int     `json:"list_body_bytes"`
	ObservationReadBytes  int     `json:"observation_read_body_bytes"`
	StableIDReadBodyBytes int     `json:"stable_id_read_body_bytes"`
	TranscriptEntries     int     `json:"transcript_entries"`
	StreamBodyBytes       int64   `json:"stream_body_bytes"`
	StreamHeaderBytes     int     `json:"stream_header_bytes"`
	ListHTTPStatus        int     `json:"list_http_status"`
	ListContentType       string  `json:"list_content_type"`
}

type prebuiltWorkscopeSourceCallCounts struct {
	PublicHTTPRequests         int `json:"public_http_requests"`
	PublicCLICommands          int `json:"public_cli_commands"`
	ListRequests               int `json:"list_requests"`
	ObservationReadRequests    int `json:"observation_read_requests"`
	StableIDReadRequests       int `json:"stable_id_read_requests"`
	RetainedStreamRequests     int `json:"retained_stream_requests"`
	CancellationStreamReads    int `json:"cancellation_stream_reads"`
	UnknownOutcomeHTTPRequests int `json:"unknown_outcome_http_requests"`
	ExternalProviderProcesses  int `json:"external_provider_processes"`
}

type prebuiltWorkscopeSourceCalls struct {
	Before prebuiltWorkscopeSourceCallCounts `json:"before_restart"`
	After  prebuiltWorkscopeSourceCallCounts `json:"after_restart"`
}

type prebuiltWorkscopeResolvedFactorySessionIDs struct {
	Before string `json:"before_restart"`
	After  string `json:"after_restart"`
}

type prebuiltWorkscopeCancellation struct {
	BeforeRequestCanceled bool `json:"before_request_canceled"`
	BeforeBodyClosed      bool `json:"before_body_closed"`
	AfterRequestCanceled  bool `json:"after_request_canceled"`
	AfterBodyClosed       bool `json:"after_body_closed"`
}

type prebuiltWorkscopeProcessMeasurement struct {
	MemoryMethod              string `json:"memory_method"`
	BeforeWorkingSetHighWater uint64 `json:"before_working_set_high_water_bytes"`
	AfterWorkingSetHighWater  uint64 `json:"after_working_set_high_water_bytes"`
	WorkingSetHighWaterBytes  uint64 `json:"working_set_high_water_bytes"`
	MaximumConcurrentChildren int    `json:"maximum_concurrent_witness_processes"`
}

type prebuiltWorkscopePerformanceReport struct {
	ArtifactSHA256            string                                     `json:"artifact_sha256"`
	ArtifactBytes             int64                                      `json:"artifact_bytes"`
	SourceFixtureSHA256       string                                     `json:"source_fixture_sha256"`
	SourceFixtureBytes        int64                                      `json:"source_fixture_bytes"`
	FixtureSHA256             string                                     `json:"fixture_sha256"`
	FixtureBytes              int64                                      `json:"fixture_bytes"`
	ProviderTranscriptSHA256  string                                     `json:"provider_transcript_sha256"`
	ProviderTranscriptBytes   int64                                      `json:"provider_transcript_bytes"`
	ProviderSessionID         string                                     `json:"provider_session_id"`
	FactorySession            string                                     `json:"factory_session_id"`
	ResolvedFactorySessionIDs prebuiltWorkscopeResolvedFactorySessionIDs `json:"resolved_factory_session_ids"`
	WorkID                    string                                     `json:"work_id"`
	WorkerSession             string                                     `json:"worker_session_id"`
	Before                    prebuiltWorkscopeMeasurements              `json:"before_restart"`
	After                     prebuiltWorkscopeMeasurements              `json:"after_restart"`
	SourceCalls               prebuiltWorkscopeSourceCalls               `json:"source_calls"`
	Cancellation              prebuiltWorkscopeCancellation              `json:"cancellation"`
	Process                   prebuiltWorkscopeProcessMeasurement        `json:"process"`
}

type prebuiltWorkscopeJourney struct {
	factorySessionSelector string
	list                   factoryapi.ListWorkerSessionsResponse
	detail                 factoryapi.WorkerSessionObservation
	transcript             factoryapi.WorkerSessionTranscriptResponse
	events                 []factoryapi.WorkerSessionEvent
	measurements           prebuiltWorkscopeMeasurements
	sourceCalls            prebuiltWorkscopeSourceCallCounts
	cancellationRequested  bool
	cancellationBodyClosed bool
}

type prebuiltWorkscopeCountedBody struct {
	reader io.Reader
	closer io.Closer
	bytes  int64
}

type prebuiltWorkscopeRequestCounter struct {
	mu    sync.Mutex
	paths map[string]int
}

func (counter *prebuiltWorkscopeRequestCounter) RoundTrip(request *http.Request) (*http.Response, error) {
	counter.mu.Lock()
	if counter.paths == nil {
		counter.paths = make(map[string]int)
	}
	counter.paths[request.URL.Path]++
	counter.mu.Unlock()
	return http.DefaultTransport.RoundTrip(request)
}

func (counter *prebuiltWorkscopeRequestCounter) count(path string) int {
	counter.mu.Lock()
	defer counter.mu.Unlock()
	return counter.paths[path]
}

func (counter *prebuiltWorkscopeRequestCounter) total() int {
	counter.mu.Lock()
	defer counter.mu.Unlock()
	total := 0
	for _, count := range counter.paths {
		total += count
	}
	return total
}

func (body *prebuiltWorkscopeCountedBody) Read(payload []byte) (int, error) {
	count, err := body.reader.Read(payload)
	body.bytes += int64(count)
	return count, err
}

func (body *prebuiltWorkscopeCountedBody) Close() error { return body.closer.Close() }

func resolvePrebuiltWorkscopeBinary(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(prebuiltWorkscopeBinaryEnv))
	if path == "" {
		path = strings.TrimSpace(os.Getenv(prebuiltWorkscopeFallbackEnv))
	}
	if path == "" {
		if strings.EqualFold(strings.TrimSpace(os.Getenv(prebuiltWorkscopeRequiredEnv)), "1") {
			t.Fatalf("%s or %s is required; this witness never builds the CLI", prebuiltWorkscopeBinaryEnv, prebuiltWorkscopeFallbackEnv)
		}
		t.Skip("prebuilt Work Session artifact unavailable; set INFINITE_YOU_INTEGRATION_BINARY")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve prebuilt Work Session artifact %q: %v", path, err)
	}
	info, err := os.Stat(absolute)
	if err != nil || info.IsDir() {
		t.Fatalf("prebuilt Work Session artifact %q is unavailable: %v", absolute, err)
	}
	return absolute
}

func startPrebuiltWorkscopeDaemon(
	t *testing.T,
	ctx context.Context,
	binaryPath, workspace string,
	environment []string,
	arguments ...string,
) *prebuiltWorkscopeDaemon {
	t.Helper()
	command := exec.CommandContext(ctx, binaryPath, arguments...)
	command.Dir = workspace
	command.Env = append([]string(nil), environment...)
	stdout, stderr := &prebuiltWorkscopeBuffer{}, &prebuiltWorkscopeBuffer{}
	command.Stdout, command.Stderr = stdout, stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start prebuilt Work Session process: %v", err)
	}
	daemon := &prebuiltWorkscopeDaemon{command: command, stdout: stdout, stderr: stderr, done: make(chan error, 1)}
	go func() { daemon.done <- command.Wait() }()
	daemon.memory = startPrebuiltWorkscopeMemoryMonitor(command.Process.Pid)
	t.Cleanup(func() { cleanupPrebuiltWorkscopeDaemon(daemon) })
	return daemon
}

func waitForPrebuiltWorkscopeTerminal(t *testing.T, ctx context.Context, daemon *prebuiltWorkscopeDaemon, serverURL, factorySessionSelector string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	var last string
	for {
		requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		statusPath := "/status"
		if factorySessionSelector != prebuiltWorkscopeFactoryID {
			statusPath = "/factory-sessions/" + url.PathEscape(factorySessionSelector) + "/status"
		}
		request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, strings.TrimSuffix(serverURL, "/")+statusPath, nil)
		if err == nil {
			response, requestErr := client.Do(request)
			if requestErr == nil {
				var status factoryapi.StatusResponse
				decodeErr := json.NewDecoder(response.Body).Decode(&status)
				response.Body.Close()
				if decodeErr == nil && response.StatusCode == http.StatusOK {
					last = fmt.Sprintf("terminal=%d failed=%d", status.Categories.Terminal, status.Categories.Failed)
					if status.Categories.Terminal > 0 && status.Categories.Failed == 0 {
						cancel()
						return
					}
				} else {
					last = fmt.Sprintf("status=%d decode=%v", response.StatusCode, decodeErr)
				}
			} else {
				last = requestErr.Error()
			}
		}
		cancel()
		select {
		case err := <-daemon.done:
			t.Fatalf("prebuilt Work Session process exited before terminal readiness: %v; last=%s\nstdout=%s\nstderr=%s", err, last, daemon.stdout.String(), daemon.stderr.String())
		case <-ctx.Done():
			t.Fatalf("wait for prebuilt Work Session readiness: %v; last=%s\nstdout=%s\nstderr=%s", ctx.Err(), last, daemon.stdout.String(), daemon.stderr.String())
		case <-deadline.C:
			t.Fatalf("prebuilt Work Session replay did not reach terminal state; last=%s\nstdout=%s\nstderr=%s", last, daemon.stdout.String(), daemon.stderr.String())
		case <-ticker.C:
		}
	}
}

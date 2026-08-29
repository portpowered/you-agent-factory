package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type s8RemoteProviderCase struct {
	repository  string
	marker      string
	sessionID   string
	output      string
	release     chan struct{}
	started     chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once
}

type s8RemoteProviderRunner struct {
	mu         sync.Mutex
	cases      []s8RemoteProviderCase
	stdout     []byte
	requestLog []platformprocess.CommandRequest
	markerLog  map[string][]string
	errorLog   []string
	active     atomic.Int32
}

func newS8RemoteProviderRunner(stdout []byte, cases ...s8RemoteProviderCase) *s8RemoteProviderRunner {
	runner := &s8RemoteProviderRunner{
		cases:     append([]s8RemoteProviderCase(nil), cases...),
		stdout:    append([]byte(nil), stdout...),
		markerLog: make(map[string][]string),
	}
	runner.reset()
	return runner
}

func (runner *s8RemoteProviderRunner) reset() {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	for index := range runner.cases {
		runner.cases[index].release = make(chan struct{})
		runner.cases[index].started = make(chan struct{})
		runner.cases[index].startOnce = sync.Once{}
		runner.cases[index].releaseOnce = sync.Once{}
	}
	runner.requestLog = nil
	runner.markerLog = make(map[string][]string)
	runner.errorLog = nil
	runner.active.Store(0)
}

func (runner *s8RemoteProviderRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requestLog)
}

func (runner *s8RemoteProviderRunner) ActiveCallCount() int {
	return int(runner.active.Load())
}

func (runner *s8RemoteProviderRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.run(ctx, request, nil)
}

func (runner *s8RemoteProviderRunner) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	return runner.run(ctx, request, observer)
}

func (runner *s8RemoteProviderRunner) run(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	runner.active.Add(1)
	defer runner.active.Add(-1)
	caseForRequest, err := runner.caseFor(request.WorkDir)
	if err != nil {
		runner.recordError(err)
		return platformprocess.CommandResult{}, err
	}
	marker, err := readS8RepositoryMarker(request.WorkDir)
	if err != nil {
		runner.recordError(err)
		return platformprocess.CommandResult{}, err
	}
	if marker != caseForRequest.marker {
		err := fmt.Errorf("S8 provider working directory %q marker = %q, want %q", request.WorkDir, marker, caseForRequest.marker)
		runner.recordError(err)
		return platformprocess.CommandResult{}, err
	}
	runner.mu.Lock()
	foreignMarkers := make([]string, 0, len(runner.cases)-1)
	for index := range runner.cases {
		other := &runner.cases[index]
		if other.repository != request.WorkDir {
			foreignMarkers = append(foreignMarkers, other.marker)
		}
	}
	runner.mu.Unlock()
	for _, foreignMarker := range foreignMarkers {
		if marker == foreignMarker {
			err := fmt.Errorf("S8 provider working directory %q observed foreign marker %q", request.WorkDir, marker)
			runner.recordError(err)
			return platformprocess.CommandResult{}, err
		}
	}
	runner.mu.Lock()
	runner.requestLog = append(runner.requestLog, cloneS8CommandRequest(request))
	runner.markerLog[request.WorkDir] = append(runner.markerLog[request.WorkDir], marker)
	runner.mu.Unlock()

	output := bytes.ReplaceAll(runner.stdout, []byte("session_fixture_codex_success"), []byte(caseForRequest.sessionID))
	output = bytes.ReplaceAll(output, []byte("Codex fixture answer COMPLETE"), []byte(caseForRequest.output))
	lineEnd := bytes.IndexByte(output, '\n')
	if lineEnd < 0 {
		lineEnd = len(output)
	} else {
		lineEnd++
	}
	if observer != nil && lineEnd > 0 {
		observer(platformprocess.OutputStreamStdout, append([]byte(nil), output[:lineEnd]...))
	}
	caseForRequest.startOnce.Do(func() { close(caseForRequest.started) })

	select {
	case <-caseForRequest.release:
	case <-ctx.Done():
		err := ctx.Err()
		runner.recordError(err)
		return platformprocess.CommandResult{}, err
	}
	if observer != nil && lineEnd < len(output) {
		observer(platformprocess.OutputStreamStdout, append([]byte(nil), output[lineEnd:]...))
	}
	return platformprocess.CommandResult{Stdout: output}, nil
}

func (runner *s8RemoteProviderRunner) caseFor(repository string) (*s8RemoteProviderCase, error) {
	for index := range runner.cases {
		if runner.cases[index].repository == repository {
			return &runner.cases[index], nil
		}
	}
	return nil, fmt.Errorf("unexpected S8 provider working directory %q", repository)
}

func (runner *s8RemoteProviderRunner) waitStarted(
	t *testing.T,
	repository string,
	routeRequests ...func() []platformprocess.CommandRequest,
) {
	t.Helper()
	caseForRequest, err := runner.caseFor(repository)
	if err != nil {
		t.Fatal(err)
	}
	watchdog := time.NewTimer(20 * time.Second)
	defer watchdog.Stop()
	select {
	case <-caseForRequest.started:
	case <-watchdog.C:
		var routed []platformprocess.CommandRequest
		if len(routeRequests) > 0 && routeRequests[0] != nil {
			routed = routeRequests[0]()
		}
		t.Fatalf("deadlock watchdog expired waiting for provider command in %q; runner calls=%d runner requests=%#v runner errors=%#v route requests=%#v", repository, runner.CallCount(), runner.requests(), runner.errors(), routed)
	}
}

func (runner *s8RemoteProviderRunner) release(t *testing.T, repository string) {
	t.Helper()
	caseForRequest, err := runner.caseFor(repository)
	if err != nil {
		t.Fatal(err)
	}
	caseForRequest.releaseOnce.Do(func() { close(caseForRequest.release) })
}

func (runner *s8RemoteProviderRunner) requests() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requestLog))
	for index, request := range runner.requestLog {
		requests[index] = cloneS8CommandRequest(request)
	}
	return requests
}

func (runner *s8RemoteProviderRunner) recordError(err error) {
	if err == nil {
		return
	}
	runner.mu.Lock()
	runner.errorLog = append(runner.errorLog, err.Error())
	runner.mu.Unlock()
}

func (runner *s8RemoteProviderRunner) errors() []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]string(nil), runner.errorLog...)
}

func (runner *s8RemoteProviderRunner) Requests() []platformprocess.CommandRequest {
	return runner.requests()
}

func (runner *s8RemoteProviderRunner) markers() map[string][]string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	markers := make(map[string][]string, len(runner.markerLog))
	for repository, values := range runner.markerLog {
		markers[repository] = append([]string(nil), values...)
	}
	return markers
}

func (runner *s8RemoteProviderRunner) releaseAll() {
	for index := range runner.cases {
		runner.cases[index].releaseOnce.Do(func() { close(runner.cases[index].release) })
	}
}

func cloneS8CommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func assertS8ProviderRequests(
	t *testing.T,
	requests []platformprocess.CommandRequest,
	markers map[string][]string,
	correlations ...s8Correlation,
) {
	t.Helper()
	if len(requests) != len(correlations) {
		t.Fatalf("provider command requests = %d, want %d: %#v", len(requests), len(correlations), requests)
	}
	assertS8ProviderMarkers(t, markers, correlations...)
	byRepository := make(map[string]s8Correlation, len(correlations))
	for _, correlation := range correlations {
		byRepository[correlation.repository] = correlation
	}
	seenRepositories := make(map[string]bool, len(correlations))
	for index, request := range requests {
		if request.Command != "codex" {
			t.Fatalf("provider request %d command = %q, want codex", index, request.Command)
		}
		expected, ok := byRepository[request.WorkDir]
		if !ok {
			t.Fatalf("provider request %d working directory = %q, want one of %#v", index, request.WorkDir, byRepository)
		}
		seenRepositories[request.WorkDir] = true
		foreign := make([]s8Correlation, 0, len(correlations)-1)
		for _, correlation := range correlations {
			if correlation.repository != expected.repository {
				foreign = append(foreign, correlation)
			}
		}
		assertS8ProviderRequest(t, request, expected, foreign...)
	}
	for _, correlation := range correlations {
		if !seenRepositories[correlation.repository] {
			t.Fatalf("provider request repositories = %#v, want repository %q", seenRepositories, correlation.repository)
		}
	}
}

func assertS8ProviderMarkers(t *testing.T, markers map[string][]string, correlations ...s8Correlation) {
	t.Helper()
	expectedRepositories := make(map[string]s8Correlation, len(correlations))
	for _, correlation := range correlations {
		expectedRepositories[correlation.repository] = correlation
	}
	if len(markers) != len(expectedRepositories) {
		t.Fatalf("provider edge marker observations = %#v, want one repository per correlation", markers)
	}
	for _, correlation := range expectedRepositories {
		observed := markers[correlation.repository]
		if len(observed) == 0 {
			t.Fatalf("provider edge observed no marker for repository %q", correlation.repository)
		}
		for _, marker := range observed {
			if marker != correlation.marker {
				t.Fatalf("provider edge marker for %q = %q, want own marker %q", correlation.repository, marker, correlation.marker)
			}
			for _, foreign := range correlations {
				if foreign.repository != correlation.repository && marker == foreign.marker {
					t.Fatalf("provider edge marker for %q = foreign marker %q", correlation.repository, marker)
				}
			}
		}
	}
}

func assertS8ProviderRequest(
	t *testing.T,
	request platformprocess.CommandRequest,
	own s8Correlation,
	foreign ...s8Correlation,
) {
	t.Helper()
	if request.Command != "codex" {
		t.Fatalf("provider request command = %q, want codex", request.Command)
	}
	requestText := strings.Join([]string{request.Command, strings.Join(request.Args, " "), string(request.Stdin), strings.Join(request.Env, "\n"), request.WorkDir}, "\n")
	if request.WorkDir != own.repository || !strings.Contains(string(request.Stdin), own.message) {
		t.Fatalf("provider request = %#v, want repository %q and message %q", request, own.repository, own.message)
	}
	for _, correlation := range foreign {
		for _, token := range correlation.tokens() {
			if own.owns(token) {
				continue
			}
			if strings.Contains(requestText, token) {
				t.Fatalf("provider request for %q contains foreign correlation %q: %#v", own.repository, token, request)
			}
		}
	}
}

func readS8RepositoryMarker(repository string) (string, error) {
	contents, err := os.ReadFile(filepath.Join(repository, "S8_MARKER"))
	if err != nil {
		return "", fmt.Errorf("read S8 repository marker in %q: %w", repository, err)
	}
	return strings.TrimSpace(string(contents)), nil
}

func readS8ProviderFixture(t *testing.T, fileName string) []byte {
	t.Helper()
	path := filepath.Join(testutil.MustRepoRoot(t), filepath.FromSlash(support.ProviderSessionFixturePath("codex", "success", fileName)))
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read S8 provider fixture %s: %v", fileName, err)
	}
	return contents
}

func writeS8CodexRollout(t *testing.T, homeDir, sessionID string, contents []byte, output string) {
	t.Helper()
	directory := filepath.Join(homeDir, ".codex", "sessions", "2026", "07", "27")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create S8 Codex session directory: %v", err)
	}
	contents = bytes.ReplaceAll(contents, []byte("Codex fixture answer COMPLETE"), []byte(output))
	path := filepath.Join(directory, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write S8 Codex rollout fixture: %v", err)
	}
}

func s8FunctionalEnvironment(homeDir string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return env
}

func decodeS8JSON(t *testing.T, stdout string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), target); err != nil {
		t.Fatalf("decode S8 public CLI JSON: %v\nstdout:\n%s", err, stdout)
	}
}

var _ platformprocess.CommandRunner = (*s8RemoteProviderRunner)(nil)

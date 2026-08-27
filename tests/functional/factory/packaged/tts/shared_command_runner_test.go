package tts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// packagedTTSEdgeOutcome is deliberately shaped like the command-runner edge,
// not the Providers service. The shared fixture therefore exercises the real
// provider adapter and replaces only the external command effect.
type packagedTTSEdgeOutcome interface {
	run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error)
	callCount() int
	lastRequest() platformprocess.CommandRequest
	lastAudioPath() string
	ownedArtifactRoot() string
}

type packagedTTSCommandOutcome struct {
	mu           sync.Mutex
	audio        []byte
	artifactRoot string
	audioPath    string
	calls        int
	failure      error
	last         platformprocess.CommandRequest
	hasLast      bool
}

func newPackagedTTSSuccessOutcome(t testing.TB, audio []byte) *packagedTTSCommandOutcome {
	t.Helper()
	return &packagedTTSCommandOutcome{
		audio:        append([]byte(nil), audio...),
		artifactRoot: t.TempDir(),
	}
}

func newPackagedTTSFailureOutcome(t testing.TB, message string) *packagedTTSCommandOutcome {
	t.Helper()
	return &packagedTTSCommandOutcome{
		artifactRoot: t.TempDir(),
		failure: providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindUnknown,
			Message: message,
		},
	}
}

func (outcome *packagedTTSCommandOutcome) run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	outcome.mu.Lock()
	outcome.calls++
	callNumber := outcome.calls
	outcome.last = clonePackagedTTSCommandRequest(request)
	outcome.hasLast = true
	failure := outcome.failure
	audio := append([]byte(nil), outcome.audio...)
	artifactRoot := outcome.artifactRoot
	outcome.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	if failure != nil {
		return platformprocess.CommandResult{ExitCode: 1}, failure
	}
	if request.Command != "codex" {
		return platformprocess.CommandResult{}, fmt.Errorf("packaged TTS command = %q, want codex", request.Command)
	}

	audioPath := filepath.Join(artifactRoot, fmt.Sprintf("audio-%d.wav", callNumber))
	if err := os.WriteFile(audioPath, audio, 0o644); err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("write packaged TTS audio artifact: %w", err)
	}
	outcome.mu.Lock()
	outcome.audioPath = audioPath
	outcome.mu.Unlock()

	encoded, err := json.Marshal([]work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		File:        audioPath,
		ContentType: "audio/wav",
		Slot:        "audio",
	}})
	if err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("marshal packaged TTS audio content: %w", err)
	}
	return platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout(string(encoded)),
	}, nil
}

func (outcome *packagedTTSCommandOutcome) callCount() int {
	if outcome == nil {
		return 0
	}
	outcome.mu.Lock()
	defer outcome.mu.Unlock()
	return outcome.calls
}

func (outcome *packagedTTSCommandOutcome) lastRequest() platformprocess.CommandRequest {
	if outcome == nil {
		return platformprocess.CommandRequest{}
	}
	outcome.mu.Lock()
	defer outcome.mu.Unlock()
	if !outcome.hasLast {
		return platformprocess.CommandRequest{}
	}
	return clonePackagedTTSCommandRequest(outcome.last)
}

func (outcome *packagedTTSCommandOutcome) lastAudioPath() string {
	if outcome == nil {
		return ""
	}
	outcome.mu.Lock()
	defer outcome.mu.Unlock()
	return outcome.audioPath
}

func (outcome *packagedTTSCommandOutcome) ownedArtifactRoot() string {
	if outcome == nil {
		return ""
	}
	return outcome.artifactRoot
}

// packagedTTSSharedCommandRunner owns the synchronized route registry at the
// exact ProviderCommandRunner edge. A route is selected from the exact child
// factory working directory in the platform request, while higher-level bound
// values are asserted from the public Work and Factory Event spine. A
// collision or ambiguous match is rejected rather than silently choosing a
// sibling scenario.
type packagedTTSSharedCommandRunner struct {
	mu      sync.Mutex
	routes  map[string]packagedTTSCommandRoute
	barrier *packagedTTSInferenceBarrier
}

type packagedTTSCommandRoute struct {
	workDir string
	outcome packagedTTSEdgeOutcome
}

func newPackagedTTSSharedCommandRunner() *packagedTTSSharedCommandRunner {
	return &packagedTTSSharedCommandRunner{
		routes: make(map[string]packagedTTSCommandRoute),
	}
}

func (runner *packagedTTSSharedCommandRunner) register(
	selector, workDir string,
	outcome packagedTTSEdgeOutcome,
) error {
	selector = strings.TrimSpace(selector)
	workDir = strings.TrimSpace(workDir)
	if selector == "" {
		return fmt.Errorf("TTS route selector is required")
	}
	if workDir == "" {
		return fmt.Errorf("TTS route %q has no working directory", selector)
	}
	if outcome == nil {
		return fmt.Errorf("TTS route %q has no command outcome", selector)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if _, exists := runner.routes[selector]; exists {
		return fmt.Errorf("TTS route selector %q is already registered", selector)
	}
	runner.routes[selector] = packagedTTSCommandRoute{
		workDir: workDir,
		outcome: outcome,
	}
	return nil
}

func (runner *packagedTTSSharedCommandRunner) unregister(selector string) error {
	selector = strings.TrimSpace(selector)
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if _, exists := runner.routes[selector]; !exists {
		return fmt.Errorf("TTS route selector %q is not registered", selector)
	}
	delete(runner.routes, selector)
	return nil
}

func (runner *packagedTTSSharedCommandRunner) routeCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.routes)
}

func (runner *packagedTTSSharedCommandRunner) setInferenceBarrier(
	barrier *packagedTTSInferenceBarrier,
) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.barrier = barrier
}

func (runner *packagedTTSSharedCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	var matched []packagedTTSCommandRoute
	for _, route := range runner.routes {
		if route.workDir == request.WorkDir {
			matched = append(matched, route)
		}
	}
	barrier := runner.barrier
	runner.mu.Unlock()

	if len(matched) == 0 {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"no packaged TTS command route matched working directory %q",
			request.WorkDir,
		)
	}
	if len(matched) != 1 {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"packaged TTS command matched %d route selectors, want one",
			len(matched),
		)
	}
	if err := barrier.wait(ctx); err != nil {
		return platformprocess.CommandResult{}, err
	}
	return matched[0].outcome.run(ctx, request)
}

func clonePackagedTTSCommandRequest(
	request platformprocess.CommandRequest,
) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

var _ platformprocess.CommandRunner = (*packagedTTSSharedCommandRunner)(nil)
var _ packagedTTSEdgeOutcome = (*packagedTTSCommandOutcome)(nil)

// packagedTTSInferenceBarrier makes the concurrent isolation witness
// deterministic: both model calls must cross the shared command boundary
// before either controlled outcome is released.
type packagedTTSInferenceBarrier struct {
	mu       sync.Mutex
	entered  int
	expected int
	released bool
	release  chan struct{}
}

func newPackagedTTSInferenceBarrier(expected int) *packagedTTSInferenceBarrier {
	return &packagedTTSInferenceBarrier{
		expected: expected,
		release:  make(chan struct{}),
	}
}

func (barrier *packagedTTSInferenceBarrier) wait(ctx context.Context) error {
	if barrier == nil {
		return nil
	}
	barrier.mu.Lock()
	barrier.entered++
	if barrier.entered >= barrier.expected && !barrier.released {
		barrier.released = true
		close(barrier.release)
	}
	barrier.mu.Unlock()
	select {
	case <-barrier.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

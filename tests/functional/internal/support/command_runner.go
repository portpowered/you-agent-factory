package support

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
)

type RecordingCommandRunner struct {
	mu       sync.Mutex
	stdout   []byte
	requests []platformprocess.CommandRequest
	calls    chan struct{}
}

type staticSuccessCommandRunner struct {
	stdout []byte
}

func NewStaticSuccessCommandRunner(stdout string) platformprocess.CommandRunner {
	return &staticSuccessCommandRunner{stdout: []byte(stdout)}
}

type gatedSuccessCommandRunner struct {
	stdout []byte
	gate   <-chan struct{}
}

// GatedFailureCommandRunner holds the first provider command until Release
// is called, then returns a terminal provider failure even if the invocation
// context was canceled. It is the controlled external command edge for stale
// result journeys: Runtime's attempt lifecycle decides whether the late
// observation is FAILED or CANCELED, while the command edge itself never
// becomes a second in-process worker implementation.
type GatedFailureCommandRunner struct {
	mu       sync.Mutex
	stdout   []byte
	gate     chan struct{}
	started  chan struct{}
	finished chan struct{}
	canceled chan platformprocess.CancellationReason
	requests []platformprocess.CommandRequest

	startOnce   sync.Once
	releaseOnce sync.Once
	finishOnce  sync.Once
	cancelOnce  sync.Once
}

// NewGatedFailureCommandRunner returns a channel-controlled provider command
// runner. The first call is observable through WaitForStart and remains held
// until Release; all calls return the same shaped failure after release.
func NewGatedFailureCommandRunner(stdout string) *GatedFailureCommandRunner {
	return &GatedFailureCommandRunner{
		stdout:   []byte(stdout),
		gate:     make(chan struct{}),
		started:  make(chan struct{}),
		finished: make(chan struct{}),
		canceled: make(chan platformprocess.CancellationReason, 1),
	}
}

// NewGatedSuccessCommandRunner returns a CommandRunner whose successful result
// is withheld until gate closes (or the invocation context ends first). It
// lets a test deterministically hold a worker dispatch open for an explicit,
// signal-based window instead of racing a fast/mocked worker's completion
// against an out-of-process observer that has to make a real HTTP round trip.
func NewGatedSuccessCommandRunner(stdout string, gate <-chan struct{}) platformprocess.CommandRunner {
	return &gatedSuccessCommandRunner{stdout: []byte(stdout), gate: gate}
}

func (r *gatedSuccessCommandRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	select {
	case <-r.gate:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	return platformprocess.CommandResult{Stdout: shapedProviderCommandStdout(req.Command, r.stdout)}, nil
}

// WaitForStart waits for the first command to reach the provider edge.
func (r *GatedFailureCommandRunner) WaitForStart(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("gated failure command runner is required")
	}
	select {
	case <-r.started:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release allows the held command to return its terminal failure.
func (r *GatedFailureCommandRunner) Release() {
	if r == nil {
		return
	}
	r.releaseOnce.Do(func() { close(r.gate) })
}

// WaitForCompletion waits until the first held command has returned.
func (r *GatedFailureCommandRunner) WaitForCompletion(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("gated failure command runner is required")
	}
	select {
	case <-r.finished:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WaitForCancellation waits until the held command observes cancellation from
// the live Workers boundary. The signal makes a functional move-versus-result
// race deterministic without releasing the late provider result early.
func (r *GatedFailureCommandRunner) WaitForCancellation(ctx context.Context) (platformprocess.CancellationReason, error) {
	if r == nil {
		return "", fmt.Errorf("gated failure command runner is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case reason := <-r.canceled:
		return reason, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// CallCount returns the number of provider command calls observed.
func (r *GatedFailureCommandRunner) CallCount() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// Requests returns detached provider command requests in observation order.
func (r *GatedFailureCommandRunner) Requests() []platformprocess.CommandRequest {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(r.requests))
	for index := range r.requests {
		requests[index] = cloneProcessCommandRequest(r.requests[index])
	}
	return requests
}

// Run records and holds a provider command, then returns the controlled late
// failure after Release even when the command context was canceled.
func (r *GatedFailureCommandRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if r == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("gated failure command runner is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	r.requests = append(r.requests, cloneProcessCommandRequest(req))
	r.mu.Unlock()
	r.startOnce.Do(func() { close(r.started) })
	go func() {
		select {
		case <-ctx.Done():
			reason := platformprocess.CancellationReasonFromContext(ctx)
			if reason == "" {
				reason = platformprocess.CancellationReasonCanceled
			}
			r.cancelOnce.Do(func() { r.canceled <- reason })
		case <-r.finished:
		}
	}()
	<-r.gate
	r.finishOnce.Do(func() { close(r.finished) })
	return platformprocess.CommandResult{
		Stdout:   shapedProviderCommandStdout(req.Command, r.stdout),
		ExitCode: 1,
	}, fmt.Errorf("controlled late provider failure")
}

// NewShapedProviderCommandRunner wraps the shared test runner so Codex and Claude
// stdout is emitted in provider-native JSONL after conductor cutover.
func NewShapedProviderCommandRunner(results ...platformprocess.CommandResult) *ShapedProviderCommandRunner {
	return &ShapedProviderCommandRunner{
		ProviderCommandRunner: testutil.NewProviderCommandRunner(results...),
	}
}

type ShapedProviderCommandRunner struct {
	*testutil.ProviderCommandRunner
}

func (r *ShapedProviderCommandRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	result, err := r.ProviderCommandRunner.Run(ctx, req)
	if err != nil {
		return result, err
	}
	result.Stdout = shapedProviderCommandStdout(req.Command, result.Stdout)
	return result, nil
}

func NewRecordingCommandRunner(stdout string) *RecordingCommandRunner {
	return &RecordingCommandRunner{
		stdout: []byte(stdout),
		calls:  make(chan struct{}, 64),
	}
}

func (r *staticSuccessCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: shapedProviderCommandStdout(req.Command, r.stdout)}, nil
}

func (r *RecordingCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.requests = append(r.requests, cloneProcessCommandRequest(req))
	r.mu.Unlock()
	select {
	case r.calls <- struct{}{}:
	default:
	}
	return platformprocess.CommandResult{Stdout: shapedProviderCommandStdout(req.Command, r.stdout)}, nil
}

func (r *RecordingCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// WaitForCall waits for the command-runner edge to observe the requested
// number of dispatches. The edge signal avoids polling an asynchronously
// scheduled provider invocation from a functional test.
func (r *RecordingCommandRunner) WaitForCall(ctx context.Context, want int) error {
	for {
		if r.CallCount() >= want {
			return nil
		}
		select {
		case <-r.calls:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (r *RecordingCommandRunner) LastRequest() platformprocess.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.requests) == 0 {
		panic("support.RecordingCommandRunner: LastRequest() called with no requests")
	}
	return cloneProcessCommandRequest(r.requests[len(r.requests)-1])
}

func (r *RecordingCommandRunner) Requests() []platformprocess.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(r.requests))
	for index := range r.requests {
		requests[index] = cloneProcessCommandRequest(r.requests[index])
	}
	return requests
}

func BuildModelWorkerConfig(provider modelprovider.Provider, model string) string {
	return fmt.Sprintf(`---
type: MODEL_WORKER
model: %s
modelProvider: %s
stopToken: COMPLETE
---
Process the input task.
`, model, provider)
}

func cloneProcessCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func shapedProviderCommandStdout(command string, stdout []byte) []byte {
	text := string(stdout)
	switch strings.ToLower(strings.TrimSpace(command)) {
	case string(modelprovider.ProviderCodex):
		return CodexSuccessStdout(text)
	case string(modelprovider.ProviderClaude):
		return ClaudeSuccessStdout(text)
	default:
		return append([]byte(nil), stdout...)
	}
}

var _ platformprocess.CommandRunner = (*RecordingCommandRunner)(nil)
var _ platformprocess.CommandRunner = (*staticSuccessCommandRunner)(nil)
var _ platformprocess.CommandRunner = (*gatedSuccessCommandRunner)(nil)
var _ platformprocess.CommandRunner = (*GatedFailureCommandRunner)(nil)
var _ platformprocess.CommandRunner = (*ShapedProviderCommandRunner)(nil)

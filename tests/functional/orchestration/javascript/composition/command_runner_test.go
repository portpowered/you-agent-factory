package composition_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// compositionCommandRunner is a provider-shaped external-effect edge. It
// observes the exact command request produced by the configured provider
// adapter, while its gates make concurrency and completion order deterministic.
// It is deliberately not a Providers service or an in-process worker fake.
type compositionCommandRunner struct {
	mu sync.Mutex

	requests []platformprocess.CommandRequest
	active   int
	peak     int

	concurrentStarted         chan struct{}
	concurrentRelease         chan struct{}
	concurrentCompleted       chan struct{}
	concurrentStartedClosed   bool
	concurrentReleased        bool
	concurrentCompletedClosed bool

	callChanged chan struct{}

	orderingExpected       map[string]struct{}
	orderingStarted        chan struct{}
	orderingStartedCount   int
	orderingStartedClosed  bool
	orderingGates          map[string]chan struct{}
	orderingReleased       map[string]bool
	orderingCompleted      map[string]chan struct{}
	orderingCompletionDone map[string]bool
	completedOrder         []string
}

func newCompositionCommandRunner() *compositionCommandRunner {
	return &compositionCommandRunner{callChanged: make(chan struct{})}
}

func (runner *compositionCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	prompt := strings.TrimSpace(string(request.Stdin))

	runner.mu.Lock()
	runner.requests = append(runner.requests, request)
	close(runner.callChanged)
	runner.callChanged = make(chan struct{})
	concurrentRelease := runner.concurrentRelease
	concurrentCall := concurrentRelease != nil && isConcurrentCompositionPrompt(prompt)
	if concurrentCall {
		runner.active++
		if runner.active > runner.peak {
			runner.peak = runner.active
		}
		if runner.active >= 2 && !runner.concurrentStartedClosed {
			close(runner.concurrentStarted)
			runner.concurrentStartedClosed = true
		}
	}
	orderingGate, orderingCall := runner.orderingGates[prompt]
	orderingCompleted := runner.orderingCompleted[prompt]
	if orderingCall {
		runner.orderingStartedCount++
		if runner.orderingStartedCount == len(runner.orderingExpected) && !runner.orderingStartedClosed {
			close(runner.orderingStarted)
			runner.orderingStartedClosed = true
		}
	}
	runner.mu.Unlock()

	if concurrentCall {
		select {
		case <-concurrentRelease:
		case <-ctx.Done():
			runner.finishConcurrentCall()
			return platformprocess.CommandResult{}, ctx.Err()
		}
		runner.finishConcurrentCall()
	}
	if orderingCall {
		select {
		case <-orderingGate:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
		runner.mu.Lock()
		runner.completedOrder = append(runner.completedOrder, prompt)
		if !runner.orderingCompletionDone[prompt] {
			close(orderingCompleted)
			runner.orderingCompletionDone[prompt] = true
		}
		runner.mu.Unlock()
	}

	if isCompositionFailurePrompt(prompt) {
		failureMessage, err := json.Marshal(map[string]any{
			"type": "error",
			"error": map[string]string{
				"type":    "unknown_error",
				"message": "provider rejected child: " + compositionFailureDiagnostic(prompt),
			},
		})
		if err != nil {
			return platformprocess.CommandResult{}, err
		}
		return platformprocess.CommandResult{
				ExitCode: 1,
				Stderr:   failureMessage,
			}, providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindUnknown,
				Message: "provider rejected child: " + compositionFailureDiagnostic(prompt),
			}
	}
	return platformprocess.CommandResult{
		Stdout: compositionProviderStdout(request.Command, "provider response: "+prompt),
	}, nil
}

func (runner *compositionCommandRunner) beginConcurrentCase() {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.concurrentStarted = make(chan struct{})
	runner.concurrentRelease = make(chan struct{})
	runner.concurrentCompleted = make(chan struct{})
	runner.concurrentStartedClosed = false
	runner.concurrentReleased = false
	runner.concurrentCompletedClosed = false
	runner.active = 0
	runner.peak = 0
}

func (runner *compositionCommandRunner) waitForConcurrent(ctx context.Context) error {
	runner.mu.Lock()
	started := runner.concurrentStarted
	runner.mu.Unlock()
	if started == nil {
		return fmt.Errorf("composition concurrency gate was not initialized")
	}
	select {
	case <-started:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runner *compositionCommandRunner) releaseConcurrent() {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.concurrentRelease != nil && !runner.concurrentReleased {
		close(runner.concurrentRelease)
		runner.concurrentReleased = true
	}
}

func (runner *compositionCommandRunner) finishConcurrentCall() {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.active--
	if runner.active == 0 && runner.concurrentStartedClosed && !runner.concurrentCompletedClosed {
		close(runner.concurrentCompleted)
		runner.concurrentCompletedClosed = true
	}
}

func (runner *compositionCommandRunner) waitForConcurrentCompletion(ctx context.Context) error {
	runner.mu.Lock()
	completed := runner.concurrentCompleted
	runner.mu.Unlock()
	if completed == nil {
		return fmt.Errorf("composition concurrency completion gate was not initialized")
	}
	select {
	case <-completed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runner *compositionCommandRunner) beginOrderingCase(labels []string) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.orderingExpected = make(map[string]struct{}, len(labels))
	runner.orderingGates = make(map[string]chan struct{}, len(labels))
	runner.orderingReleased = make(map[string]bool, len(labels))
	runner.orderingCompleted = make(map[string]chan struct{}, len(labels))
	runner.orderingCompletionDone = make(map[string]bool, len(labels))
	for _, label := range labels {
		runner.orderingExpected[label] = struct{}{}
		runner.orderingGates[label] = make(chan struct{})
		runner.orderingCompleted[label] = make(chan struct{})
	}
	runner.orderingStarted = make(chan struct{})
	runner.orderingStartedCount = 0
	runner.orderingStartedClosed = false
	runner.completedOrder = nil
}

func (runner *compositionCommandRunner) waitForOrderingStarted(ctx context.Context) error {
	runner.mu.Lock()
	started := runner.orderingStarted
	runner.mu.Unlock()
	if started == nil {
		return fmt.Errorf("composition ordering gate was not initialized")
	}
	select {
	case <-started:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runner *compositionCommandRunner) releaseLabel(label string) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	gate, ok := runner.orderingGates[label]
	if !ok || runner.orderingReleased[label] {
		return
	}
	close(gate)
	runner.orderingReleased[label] = true
}

func (runner *compositionCommandRunner) waitForLabel(ctx context.Context, label string) error {
	runner.mu.Lock()
	completed, ok := runner.orderingCompleted[label]
	runner.mu.Unlock()
	if !ok {
		return fmt.Errorf("composition ordering label %q was not initialized", label)
	}
	select {
	case <-completed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runner *compositionCommandRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *compositionCommandRunner) waitForCallCount(ctx context.Context, want int) error {
	for {
		runner.mu.Lock()
		if len(runner.requests) >= want {
			runner.mu.Unlock()
			return nil
		}
		changed := runner.callChanged
		runner.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (runner *compositionCommandRunner) activeCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.active
}

func (runner *compositionCommandRunner) peakActive() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.peak
}

func (runner *compositionCommandRunner) completionOrder() []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]string(nil), runner.completedOrder...)
}

func isConcurrentCompositionPrompt(prompt string) bool {
	return prompt == "summarize alpha" || prompt == "summarize beta"
}

func isCompositionFailurePrompt(prompt string) bool {
	lower := strings.ToLower(prompt)
	return strings.HasPrefix(lower, "fail:") || strings.Contains(lower, "force provider failure")
}

func compositionFailureDiagnostic(prompt string) string {
	if strings.HasPrefix(strings.ToLower(prompt), "fail:") {
		return prompt[len("fail:"):]
	}
	return strings.Replace(prompt, "force provider failure", "provider failure", 1)
}

func compositionProviderStdout(command, result string) []byte {
	if strings.EqualFold(strings.TrimSpace(command), string(modelprovider.ProviderClaude)) {
		return support.ClaudeSuccessStdout(result)
	}
	return support.CodexSuccessStdout(result)
}

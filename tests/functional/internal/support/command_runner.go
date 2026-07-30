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
}

type staticSuccessCommandRunner struct {
	stdout []byte
}

func NewStaticSuccessCommandRunner(stdout string) platformprocess.CommandRunner {
	return &staticSuccessCommandRunner{stdout: []byte(stdout)}
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
	return &RecordingCommandRunner{stdout: []byte(stdout)}
}

func (r *staticSuccessCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: shapedProviderCommandStdout(req.Command, r.stdout)}, nil
}

func (r *RecordingCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests = append(r.requests, cloneProcessCommandRequest(req))
	return platformprocess.CommandResult{Stdout: shapedProviderCommandStdout(req.Command, r.stdout)}, nil
}

func (r *RecordingCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
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
var _ platformprocess.CommandRunner = (*ShapedProviderCommandRunner)(nil)

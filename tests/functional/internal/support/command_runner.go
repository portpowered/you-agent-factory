package support

import (
	"context"
	"fmt"
	"sync"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

type RecordingCommandRunner struct {
	mu       sync.Mutex
	stdout   []byte
	requests []workers.CommandRequest
}

type staticSuccessCommandRunner struct {
	stdout []byte
}

func NewStaticSuccessCommandRunner(stdout string) workers.CommandRunner {
	return &staticSuccessCommandRunner{stdout: []byte(stdout)}
}

func NewRecordingCommandRunner(stdout string) *RecordingCommandRunner {
	return &RecordingCommandRunner{stdout: []byte(stdout)}
}

func (r *staticSuccessCommandRunner) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{Stdout: append([]byte(nil), r.stdout...)}, nil
}

func (r *RecordingCommandRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests = append(r.requests, workers.CommandRequest(workerexecution.CloneSubprocessExecutionRequest(req)))
	return workers.CommandResult{Stdout: append([]byte(nil), r.stdout...)}, nil
}

func (r *RecordingCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func (r *RecordingCommandRunner) LastRequest() workers.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.requests) == 0 {
		panic("support.RecordingCommandRunner: LastRequest() called with no requests")
	}
	return workers.CommandRequest(workerexecution.CloneSubprocessExecutionRequest(r.requests[len(r.requests)-1]))
}

func BuildModelWorkerConfig(provider modelprovider.ID, model string) string {
	return fmt.Sprintf(`---
type: MODEL_WORKER
model: %s
modelProvider: %s
stopToken: COMPLETE
---
Process the input task.
`, model, provider)
}

var _ workers.CommandRunner = (*RecordingCommandRunner)(nil)
var _ workers.CommandRunner = (*staticSuccessCommandRunner)(nil)

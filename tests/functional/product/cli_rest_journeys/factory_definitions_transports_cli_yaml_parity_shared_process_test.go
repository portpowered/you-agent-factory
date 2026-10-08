package cli_rest_journeys_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var yamlParityCLIProcess support.ApplicationProcess
var yamlParityCommands = &sourceCommandObserver{calls: make(map[string]int), sessions: make(map[string]platformprocess.CommandRunner)}
var yamlParityAPI = support.NewProcessAPIServer()

// Each invocation owns its working directory, so rejection observations remain
// attributable while independent source cases run concurrently.
type sourceCommandObserver struct {
	mu       sync.Mutex
	calls    map[string]int
	sessions map[string]platformprocess.CommandRunner
}

func (r *sourceCommandObserver) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if _, ok := ctx.Value(validationObservationKey{}).(*validationObservation); ok {
		return (validationCommandRunner{}).Run(ctx, req)
	}
	r.mu.Lock()
	runner := r.sessions[req.ExecutionScopeID]
	if runner != nil {
		r.mu.Unlock()
		return runner.Run(ctx, req)
	}
	r.calls[filepath.Clean(req.WorkDir)]++
	r.mu.Unlock()
	return platformprocess.CommandResult{}, fmt.Errorf("unexpected provider dispatch for rejected authored source: %s", req.WorkDir)
}

func (r *sourceCommandObserver) registerSession(t *testing.T, sessionID string, runner platformprocess.CommandRunner) {
	t.Helper()
	r.mu.Lock()
	r.sessions[sessionID] = runner
	r.mu.Unlock()
	t.Cleanup(func() {
		r.mu.Lock()
		delete(r.sessions, sessionID)
		r.mu.Unlock()
	})
}

func (r *sourceCommandObserver) callCount(directory string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[filepath.Clean(directory)]
}

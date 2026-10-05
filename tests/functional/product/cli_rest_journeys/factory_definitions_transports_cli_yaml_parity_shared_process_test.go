package cli_rest_journeys_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
func initializeFactorydefinitionstransportscliyamlparityFixture(t *testing.T) {
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner:     yamlParityCommands,
		APIServerStarter:          yamlParityAPI.Start,
		FactorySessionIDGenerator: uuid.NewString,
		WorkRequestIDGenerator:    uuid.NewString,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build YAML parity CLI process: %v\n", err)
		t.Fatal("customer fixture setup failed; see preceding diagnostic")
	}
	yamlParityCLIProcess = process
	t.Cleanup(func() {
		exitCode := 0
		//nolint:testsleep // This deadline bounds process teardown after all scenario-owned commands have joined.
		closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := process.Close(closeContext); err != nil {
			fmt.Fprintf(os.Stderr, "close YAML parity CLI process: %v\n", err)
			exitCode = 1
		}
		if exitCode != 0 {
			t.Error("customer fixture cleanup failed; see preceding diagnostic")
		}
	})
}
func resetfactorydefinitionstransportscliyamlparity1State() {
	var freshYamlParityCLIProcess support.ApplicationProcess
	yamlParityCLIProcess = freshYamlParityCLIProcess
	var freshYamlParityCommands = &sourceCommandObserver{calls: make(map[string]int), sessions: make(map[string]platformprocess.CommandRunner)}
	yamlParityCommands = freshYamlParityCommands
	var freshYamlParityAPI = support.NewProcessAPIServer()
	yamlParityAPI = freshYamlParityAPI
}

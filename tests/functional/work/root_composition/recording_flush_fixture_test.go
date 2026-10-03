package root_composition_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type flushCommandRunner struct {
	run func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error)
}

func (runner flushCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.run(ctx, request)
}

type flushCase struct {
	name, dir, home, sessionID             string
	port                                   int
	api                                    *support.ProcessAPIServer
	entered, release, completed            chan struct{}
	entryOnce, releaseOnce, completionOnce sync.Once
	hold                                   atomic.Bool
}

func newFlushCases(t *testing.T) []*flushCase {
	t.Helper()
	var cases []*flushCase
	for index, name := range []string{"successful storage", "failed storage", "disabled storage"} {
		dir := support.ScaffoldFactory(t, recordingsActivationFactoryConfig())
		support.WriteAgentConfig(t, dir, "mock-worker", support.BuildModelWorkerConfig("codex", "test-model"))
		support.WriteWorkstationConfig(t, dir, "process-task", "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
		cases = append(cases, &flushCase{
			name: name, dir: dir, home: t.TempDir(), sessionID: uuid.NewString(),
			port: 48101 + index, api: support.NewProcessAPIServer(),
			entered: make(chan struct{}), release: make(chan struct{}), completed: make(chan struct{}),
		})
	}
	return cases
}

func (scenario *flushCase) releaseWrite() {
	scenario.releaseOnce.Do(func() { close(scenario.release) })
}

func (scenario *flushCase) write(path string, data []byte) error {
	// Initial host recording must finish before readiness; only writes after
	// provider execution are held, so completed Work can be observed live.
	if !scenario.hold.Load() {
		return os.WriteFile(path, data, 0o600)
	}
	scenario.entryOnce.Do(func() { close(scenario.entered) })
	<-scenario.release
	defer scenario.completionOnce.Do(func() { close(scenario.completed) })
	if scenario.name == "failed storage" {
		return errors.New("controlled recording write failure")
	}
	return os.WriteFile(path, data, 0o600)
}

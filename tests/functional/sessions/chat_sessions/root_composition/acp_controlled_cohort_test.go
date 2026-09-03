// Functional owner: sessions/chat_sessions/root_composition.
package root_composition_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const controlledACPFactory = "@you/goal"

// controlledACPCohort is the package's immutable-profile process fixture.
// Every customer scenario owns a distinct Chat Session, Factory Session, and
// working root while reusing this one root-built application graph.
type controlledACPCohort struct {
	home                  string
	process               support.ApplicationProcess
	runner                *controlledACPCommandRunner
	factorySessionIDCalls atomic.Int32
	workingDirectoryRoot  string
}

var controlledCohortState struct {
	sync.Mutex
	cohort *controlledACPCohort
	err    error
}

func controlledACPHome(t *testing.T) string {
	t.Helper()
	return controlledACPCohortForTest(t).home
}

func controlledACPWorkingDirectory(t *testing.T, name string) string {
	t.Helper()
	cohort := controlledACPCohortForTest(t)
	return controlledACPWorkingDirectoryForCohort(t, cohort, name)
}

func controlledACPWorkingDirectoryForCohort(t *testing.T, cohort *controlledACPCohort, name string) string {
	t.Helper()
	workingDirectory := chatMkdirTemp(t, "controlled ACP "+name+" working directory", cohort.workingDirectoryRoot, name+"-")
	seedProjectPackagedFactory(t, workingDirectory, controlledACPFactory)
	return workingDirectory
}

func controlledACPServer(t *testing.T) support.ACPServer {
	t.Helper()
	cohort := controlledACPCohortForTest(t)
	return controlledACPServerForCohort(t, cohort)
}

func controlledACPServerForCohort(t *testing.T, cohort *controlledACPCohort) support.ACPServer {
	t.Helper()
	server := cohort.process.ACPServer()
	if server == nil {
		t.Fatal("controlled ACP cohort Process.ACPServer() returned nil")
	}
	registerChatACPServerHome(server, cohort.home)
	return server
}

// controlledACPCommandRunner routes from the provider request itself. The
// request contains the current user turn, so the result does not depend on
// which compatible scenario ran first or how many provider calls a previous
// turn made. The busy route is armed only by its owning witness and can be
// released by the witness without changing the process edge.
type controlledACPCommandRunner struct {
	mu              sync.Mutex
	requests        []process.CommandRequest
	busyStarted     chan struct{}
	busyRelease     chan struct{}
	busyActive      bool
	busyStartedOnce sync.Once
	busyReleaseOnce sync.Once
}

func (runner *controlledACPCommandRunner) Run(
	ctx context.Context,
	request process.CommandRequest,
) (process.CommandResult, error) {
	runner.mu.Lock()
	runner.requests = append(runner.requests, request)
	busyStarted := runner.busyStarted
	busyRelease := runner.busyRelease
	busyActive := runner.busyActive
	runner.mu.Unlock()

	prompt := string(request.Stdin)
	switch {
	case strings.Contains(prompt, "[cohort-failure]"):
		return controlledACPResult("not a decision envelope"), nil
	case strings.Contains(prompt, "[cohort-busy-concurrent]"):
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"busy concurrent route"}`), nil
	case strings.Contains(prompt, "[cohort-busy-later]"):
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"busy later route"}`), nil
	case strings.Contains(prompt, "[cohort-busy]") && busyActive:
		runner.busyStartedOnce.Do(func() { close(busyStarted) })
		select {
		case <-busyRelease:
		case <-ctx.Done():
			return process.CommandResult{}, ctx.Err()
		}
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"busy first route"}`), nil
	case strings.Contains(prompt, "pursue the third goal"):
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"third turn answer"}`), nil
	case strings.Contains(prompt, "pursue the second goal"):
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"second turn answer"}`), nil
	case strings.Contains(prompt, "pursue the first goal"):
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"first turn answer"}`), nil
	case strings.Contains(prompt, "please pursue this goal"):
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"goal genuinely completed through you server acp"}`), nil
	default:
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"goal reached over ACP"}`), nil
	}
}

func (runner *controlledACPCommandRunner) requestCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *controlledACPCommandRunner) requestCountContaining(marker string) int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	count := 0
	for _, request := range runner.requests {
		if strings.Contains(string(request.Stdin), marker) {
			count++
		}
	}
	return count
}

func controlledACPResult(output string) process.CommandResult {
	return process.CommandResult{Stdout: support.CodexSuccessStdout(output)}
}

func (runner *controlledACPCommandRunner) armBusy() (<-chan struct{}, func()) {
	runner.mu.Lock()
	runner.busyStarted = make(chan struct{})
	runner.busyRelease = make(chan struct{})
	runner.busyActive = true
	runner.busyStartedOnce = sync.Once{}
	runner.busyReleaseOnce = sync.Once{}
	started := runner.busyStarted
	release := runner.busyRelease
	runner.mu.Unlock()

	return started, func() {
		runner.busyReleaseOnce.Do(func() {
			close(release)
			runner.mu.Lock()
			runner.busyActive = false
			runner.mu.Unlock()
		})
	}
}

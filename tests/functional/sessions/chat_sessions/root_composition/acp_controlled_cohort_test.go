// Functional owner: sessions/chat_sessions/root_composition.
package root_composition_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const controlledACPFactory = "@you/goal"

// controlledACPCohort is an immutable-profile process fixture. The shared
// package cohort is used for ACP witnesses that do not activate a Factory
// runtime; activation-owning witnesses use newControlledACPCohort so their
// retained ~default Factory Definitions binding cannot leak into another
// scenario. Both paths use the same root.BuildProcess composition and the
// same request-keyed command-runner edge.
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

// newControlledACPCohort builds one root process for a scenario whose real
// Factory activation remains retained by the on-demand ACP target. The
// immutable catalog/profile seed is shared, but the command home, process, and
// working root stay scenario-local: the production runtime currently binds Factory
// Definitions under the fixed ~default session scope and the public ACP close
// path cannot close a terminalized session, so sharing that retained
// activation would make later tests fail with dependency_unavailable.
func newControlledACPCohort(t *testing.T, name string) *controlledACPCohort {
	t.Helper()
	home := seedACPActivationCommandHomeForTest(
		t,
		"controlled ACP "+name+" initialization home",
		"you-chat-sessions-"+name+"-home-",
	)
	workingDirectoryRoot := chatMkdirTemp(
		t,
		"controlled ACP "+name+" working roots",
		"",
		"you-chat-sessions-"+name+"-",
	)
	if err := os.MkdirAll(workingDirectoryRoot, 0o755); err != nil {
		t.Fatalf("create controlled ACP %s working roots: %v", name, err)
	}

	runner := &controlledACPCommandRunner{}

	cohort := &controlledACPCohort{
		home:                 home,
		runner:               runner,
		workingDirectoryRoot: workingDirectoryRoot,
	}
	process, err := buildChatProcess(t, "controlled ACP "+name, serviceedges.Edges{
		ProviderCommandRunner: runner,
		FactorySessionIDGenerator: func() string {
			n := cohort.factorySessionIDCalls.Add(1)
			return fmt.Sprintf("acp-%s-factory-session-%d", name, n)
		},
	})
	if err != nil {
		t.Fatalf("build controlled ACP %s process: %v", name, err)
	}
	cohort.process = process
	t.Cleanup(func() {
		if err := closeChatProcess(cohort.process); err != nil {
			t.Errorf("close controlled ACP %s process: %v", name, err)
		}
	})
	return cohort
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

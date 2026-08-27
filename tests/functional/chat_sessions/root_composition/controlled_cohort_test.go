package root_composition_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// TestMain closes the package-scoped process only after every test invocation
// has stopped. Per-test ACP command executions close their own pipes and
// contexts, while this final close releases runtimes retained by the shared
// process before its fixed home is removed.
func TestMain(m *testing.M) {
	code := m.Run()

	closeCatalogCohort()

	controlledCohortState.Lock()
	cohort := controlledCohortState.cohort
	controlledCohortState.Unlock()
	if cohort != nil {
		if err := cohort.process.Close(context.Background()); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "controlled ACP cohort close: %v\n", err)
		}
		if err := os.RemoveAll(cohort.home); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "controlled ACP cohort home cleanup: %v\n", err)
		}
	}

	os.Exit(code)
}

func controlledACPCohortForTest(t *testing.T) *controlledACPCohort {
	t.Helper()

	controlledCohortState.Lock()
	defer controlledCohortState.Unlock()
	if controlledCohortState.cohort == nil && controlledCohortState.err == nil {
		home, err := os.MkdirTemp("", "you-chat-sessions-cohort-")
		if err != nil {
			controlledCohortState.err = fmt.Errorf("create controlled ACP cohort home: %w", err)
		} else {
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			workingDirectoryRoot := filepath.Join(home, "workdirs")
			if err := os.MkdirAll(workingDirectoryRoot, 0o755); err != nil {
				controlledCohortState.err = fmt.Errorf("create controlled ACP cohort workdirs: %w", err)
				_ = os.RemoveAll(home)
			} else {
				runner := &controlledACPCommandRunner{}
				seedInstalledPackagedFactory(t, home, controlledACPFactory)
				support.SeedACPAgentProfile(t, home, "factory:"+controlledACPFactory, []string{"factory:" + controlledACPFactory})

				cohort := &controlledACPCohort{
					home:                 home,
					runner:               runner,
					workingDirectoryRoot: workingDirectoryRoot,
				}
				process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
					ProviderCommandRunner: runner,
					FactorySessionIDGenerator: func() string {
						n := cohort.factorySessionIDCalls.Add(1)
						return fmt.Sprintf("acp-cohort-factory-session-%d", n)
					},
				})
				if err != nil {
					controlledCohortState.err = fmt.Errorf("build controlled ACP cohort process: %w", err)
					_ = os.RemoveAll(home)
				} else {
					cohort.process = process
					controlledCohortState.cohort = cohort
				}
			}
		}
	}
	if controlledCohortState.err != nil {
		t.Fatalf("controlled ACP cohort: %v", controlledCohortState.err)
	}

	cohort := controlledCohortState.cohort
	t.Setenv("HOME", cohort.home)
	t.Setenv("USERPROFILE", cohort.home)
	return cohort
}

// newControlledACPCohort builds one fixed-profile root for a scenario whose
// real Factory activation remains retained by the on-demand ACP target. The
// production runtime currently binds Factory Definitions under the fixed
// ~default session scope and the public ACP close path cannot close a
// terminalized session, so sharing that retained activation across scenarios
// would make later tests fail with dependency_unavailable. This isolated
// process is the smallest faithful boundary until the production activation
// owner gains a supported release/reopen capability.
func newControlledACPCohort(t *testing.T, name string) *controlledACPCohort {
	t.Helper()
	home, err := os.MkdirTemp("", "you-chat-sessions-"+name+"-")
	if err != nil {
		t.Fatalf("create controlled ACP %s home: %v", name, err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	workingDirectoryRoot := filepath.Join(home, "workdirs")
	if err := os.MkdirAll(workingDirectoryRoot, 0o755); err != nil {
		_ = os.RemoveAll(home)
		t.Fatalf("create controlled ACP %s workdirs: %v", name, err)
	}

	runner := &controlledACPCommandRunner{}
	seedInstalledPackagedFactory(t, home, controlledACPFactory)
	support.SeedACPAgentProfile(t, home, "factory:"+controlledACPFactory, []string{"factory:" + controlledACPFactory})

	cohort := &controlledACPCohort{
		home:                 home,
		runner:               runner,
		workingDirectoryRoot: workingDirectoryRoot,
	}
	cohort.process, err = support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: runner,
		FactorySessionIDGenerator: func() string {
			n := cohort.factorySessionIDCalls.Add(1)
			return fmt.Sprintf("acp-%s-factory-session-%d", name, n)
		},
	})
	if err != nil {
		_ = os.RemoveAll(home)
		t.Fatalf("build controlled ACP %s process: %v", name, err)
	}
	t.Cleanup(func() {
		if err := cohort.process.Close(context.Background()); err != nil {
			t.Errorf("close controlled ACP %s process: %v", name, err)
		}
		if err := os.RemoveAll(home); err != nil {
			t.Errorf("remove controlled ACP %s home: %v", name, err)
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
	directory, err := os.MkdirTemp(cohort.workingDirectoryRoot, name+"-")
	if err != nil {
		t.Fatalf("create controlled ACP working directory: %v", err)
	}
	return directory
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

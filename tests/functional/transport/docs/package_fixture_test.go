package docs_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const docsProcessCloseTimeout = 5 * time.Second
const docsProviderResultCount = 64

var documentationProcesses *documentationProcessFixture

// TestMain owns the one immutable root-built process used by every docs
// scenario. Inputs, output streams, homes, and working roots remain
// invocation-local so repeated commands cannot borrow state from one another.
func TestMain(m *testing.M) {
	providerRunner := support.NewShapedProviderCommandRunner(documentationProviderResults()...)
	ledger := newDocumentationLifecycleLedger()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: providerRunner,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build docs functional process: %v\n", err)
		os.Exit(1)
	}
	ledger.processStarted()
	documentationProcesses = &documentationProcessFixture{
		process:        process,
		providerRunner: providerRunner,
		ledger:         ledger,
	}

	exitCode := m.Run()
	closeContext, cancel := context.WithTimeout(context.Background(), docsProcessCloseTimeout)
	closeErr := process.Close(closeContext)
	cancel()
	ledger.processClosed()
	if closeErr != nil {
		fmt.Fprintf(os.Stderr, "close docs functional process: %v\n", closeErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	if ledgerErr := ledger.assertClean(); ledgerErr != nil {
		fmt.Fprintf(os.Stderr, "docs lifecycle ledger: %v\n", ledgerErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	fmt.Fprintf(os.Stderr, "docs lifecycle ledger: %s\n", ledger.summary())
	os.Exit(exitCode)
}

func documentationProviderResults() []platformprocess.CommandResult {
	results := make([]platformprocess.CommandResult, docsProviderResultCount)
	for index := range results {
		results[index] = platformprocess.CommandResult{
			Stdout: []byte("documentation example completed. COMPLETE"),
		}
	}
	return results
}

type documentationProcessFixture struct {
	process        support.ApplicationProcess
	providerRunner *support.ShapedProviderCommandRunner
	ledger         *documentationLifecycleLedger
}

func documentationProcess(t testing.TB) *documentationProcessFixture {
	t.Helper()
	if documentationProcesses == nil {
		t.Fatal("docs functional process was not initialized by TestMain")
	}
	return documentationProcesses
}

func (fixture *documentationProcessFixture) tempDir(t testing.TB) string {
	t.Helper()
	directory, err := os.MkdirTemp("", "functional-docs-")
	if err != nil {
		t.Fatalf("create docs temporary root: %v", err)
	}
	if err := fixture.ledger.registerRoot(directory); err != nil {
		_ = os.RemoveAll(directory)
		t.Fatalf("register docs temporary root %q: %v", directory, err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Errorf("remove docs temporary root %q: %v", directory, err)
			return
		}
		if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				t.Errorf("docs temporary root %q still exists after cleanup", directory)
			} else {
				t.Errorf("stat removed docs temporary root %q: %v", directory, err)
			}
			return
		}
		fixture.ledger.rootRemoved(directory)
	})
	return directory
}

func (fixture *documentationProcessFixture) freshEnvironment(t testing.TB, base []string) []string {
	t.Helper()
	home := fixture.tempDir(t)
	environment := make([]string, 0, len(base)+4)
	for _, entry := range base {
		name := strings.SplitN(entry, "=", 2)[0]
		switch {
		case strings.EqualFold(name, "HOME"),
			strings.EqualFold(name, "USERPROFILE"),
			strings.EqualFold(name, "XDG_CACHE_HOME"),
			strings.EqualFold(name, "INFINITE_YOU_OMNIVOICE_CACHE_DIR"):
			continue
		default:
			environment = append(environment, entry)
		}
	}
	return append(environment,
		"HOME="+home,
		"USERPROFILE="+home,
		"XDG_CACHE_HOME="+filepath.Join(home, "cache"),
		"INFINITE_YOU_OMNIVOICE_CACHE_DIR="+filepath.Join(home, "omnivoice-cache"),
	)
}

type documentationLifecycleLedger struct {
	mu                 sync.Mutex
	processStarts      int
	processCloses      int
	activeInvocations  int
	activeOutputBuffer int
	roots              map[string]bool
}

func newDocumentationLifecycleLedger() *documentationLifecycleLedger {
	return &documentationLifecycleLedger{roots: make(map[string]bool)}
}

func (ledger *documentationLifecycleLedger) processStarted() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processStarts++
}

func (ledger *documentationLifecycleLedger) processClosed() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processCloses++
}

func (ledger *documentationLifecycleLedger) beginInvocation() func() {
	ledger.mu.Lock()
	ledger.activeInvocations++
	ledger.activeOutputBuffer++
	ledger.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			ledger.mu.Lock()
			ledger.activeInvocations--
			ledger.activeOutputBuffer--
			ledger.mu.Unlock()
		})
	}
}

func (ledger *documentationLifecycleLedger) registerRoot(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("normalize docs root %q: %w", path, err)
	}
	absolute = filepath.Clean(absolute)
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if _, exists := ledger.roots[absolute]; exists {
		return fmt.Errorf("docs root %q registered twice", absolute)
	}
	ledger.roots[absolute] = false
	return nil
}

func (ledger *documentationLifecycleLedger) rootRemoved(path string) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if _, exists := ledger.roots[filepath.Clean(absolute)]; exists {
		ledger.roots[filepath.Clean(absolute)] = true
	}
}

func (ledger *documentationLifecycleLedger) assertClean() error {
	ledger.mu.Lock()
	processStarts := ledger.processStarts
	processCloses := ledger.processCloses
	activeInvocations := ledger.activeInvocations
	activeOutputBuffer := ledger.activeOutputBuffer
	roots := make(map[string]bool, len(ledger.roots))
	for path, removed := range ledger.roots {
		roots[path] = removed
	}
	ledger.mu.Unlock()

	var problems []error
	if processStarts != 1 {
		problems = append(problems, fmt.Errorf("process starts = %d, want 1", processStarts))
	}
	if processCloses != 1 {
		problems = append(problems, fmt.Errorf("process closes = %d, want 1", processCloses))
	}
	if activeInvocations != 0 {
		problems = append(problems, fmt.Errorf("active invocations = %d, want 0", activeInvocations))
	}
	if activeOutputBuffer != 0 {
		problems = append(problems, fmt.Errorf("active output buffers = %d, want 0", activeOutputBuffer))
	}
	for path, removed := range roots {
		if !removed {
			problems = append(problems, fmt.Errorf("docs temporary root %q was not removed", path))
		}
	}
	return errors.Join(problems...)
}

func (ledger *documentationLifecycleLedger) summary() string {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	removed := 0
	for _, rootRemoved := range ledger.roots {
		if rootRemoved {
			removed++
		}
	}
	return fmt.Sprintf(
		"process_starts=%d process_closes=%d active_invocations=%d active_output_buffers=%d active_sessions=0 active_routes=0 roots_removed=%d/%d",
		ledger.processStarts, ledger.processCloses, ledger.activeInvocations,
		ledger.activeOutputBuffer, removed, len(ledger.roots),
	)
}

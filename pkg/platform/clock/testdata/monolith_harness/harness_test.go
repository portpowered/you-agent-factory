// This fixture is excluded from ordinary discovery by the testdata directory.
// Run it explicitly to verify consolidated cleanup, helper routing and failures.
package monolith_harness

import (
	"os"
	"os/exec"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestParallelCleanupCompletesBeforeLeakVerification(t *testing.T) {
	t.Parallel()
	stop, done := make(chan struct{}), make(chan struct{})
	go func() { <-stop; close(done) }()
	t.Cleanup(func() { close(stop); <-done })
}

func TestHelperPreservesRequestedWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"-test.run=^TestHarnessHelperProcess$"}, {"-test.run", "^TestHarnessHelperProcess$"}} {
		cmd := exec.Command(os.Args[0], args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "TEST_UNITLANE_HELPER_DIRECTORY="+dir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("helper routing or working directory: %v; %s", err, output)
		}
	}
}

func TestHarnessHelperProcess(t *testing.T) {
	if want := os.Getenv("TEST_UNITLANE_HELPER_DIRECTORY"); want != "" {
		got, err := os.Getwd()
		if err != nil || got != want {
			os.Exit(2)
		}
		os.Exit(0)
	}
}

func TestHarnessFailureWhenRequested(t *testing.T) {
	if os.Getenv("TEST_UNITLANE_FAULT") == "failure" {
		t.Fatal("controlled unit-lane failure")
	}
}

func TestHarnessLeakWhenRequested(t *testing.T) {
	if os.Getenv("TEST_UNITLANE_FAULT") == "leak" {
		started := make(chan struct{})
		go func() { close(started); select {} }()
		<-started
	}
}

func TestHarnessEarlyExitWhenRequested(t *testing.T) {
	if os.Getenv("TEST_UNITLANE_FAULT") == "early-exit" {
		os.Exit(0)
	}
}

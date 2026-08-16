package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDefaultPostRunCleanupGracePeriod(t *testing.T) {
	if defaultPostRunCleanupGracePeriod != 10*time.Second {
		t.Fatalf("defaultPostRunCleanupGracePeriod = %v, want 10s", defaultPostRunCleanupGracePeriod)
	}
}

func TestPostRunCleanupGracePeriod_TestHookOverridesDefault(t *testing.T) {
	t.Cleanup(func() {
		postRunCleanupGracePeriodForTest = 0
	})

	if postRunCleanupGracePeriod() != defaultPostRunCleanupGracePeriod {
		t.Fatalf("postRunCleanupGracePeriod() = %v, want default %v", postRunCleanupGracePeriod(), defaultPostRunCleanupGracePeriod)
	}

	postRunCleanupGracePeriodForTest = 25 * time.Millisecond
	if postRunCleanupGracePeriod() != 25*time.Millisecond {
		t.Fatalf("postRunCleanupGracePeriod() = %v, want test override 25ms", postRunCleanupGracePeriod())
	}
}

type deterministicCommandTimerClock struct {
	now   time.Time
	timer chan time.Time
}

func (clock *deterministicCommandTimerClock) Now() time.Time {
	return clock.now
}

func (clock *deterministicCommandTimerClock) After(time.Duration) <-chan time.Time {
	return clock.timer
}

func (clock *deterministicCommandTimerClock) Advance(duration time.Duration) {
	clock.now = clock.now.Add(duration)
	clock.timer <- clock.now
}

func TestWaitForCommandExitUsesInjectedTimerWhenClockNowIsStatic(t *testing.T) {
	clock := &deterministicCommandTimerClock{
		now:   time.Unix(42, 0),
		timer: make(chan time.Time, 1),
	}
	waitCh := make(chan error)
	result := make(chan bool, 1)
	go func() {
		result <- waitForCommandExit(waitCh, clock, time.Second)
	}()

	select {
	case got := <-result:
		t.Fatalf("waitForCommandExit returned %t before injected timer advanced", got)
	case <-time.After(50 * time.Millisecond):
	}

	clock.Advance(time.Second)
	select {
	case got := <-result:
		if got {
			t.Fatal("waitForCommandExit returned true after injected grace timer fired")
		}
	case <-time.After(time.Second):
		t.Fatal("waitForCommandExit did not observe the injected grace timer")
	}
}

func TestExecCommandRunner_RunStreamingBoundsRetainedOutputAndForwardsAllChunks(t *testing.T) {
	requireProcessIntegration(t)
	const (
		stdoutMarker = "streaming stdout complete"
		stderrMarker = "authentication failed after large output"
	)
	var observedStdout, observedStderr int
	result, err := testExecCommandRunner(t, nil).RunStreaming(
		context.Background(),
		CommandRequest{
			Command: os.Args[0],
			Args: []string{
				"-test.run=TestExecCommandRunner_HelperProcess",
				"--",
				"streaming-output",
			},
			Env: append(os.Environ(), "GO_WANT_COMMAND_HELPER=1"),
		},
		func(stream string, chunk []byte) {
			switch stream {
			case OutputStreamStdout:
				observedStdout += len(chunk)
			case OutputStreamStderr:
				observedStderr += len(chunk)
			}
		},
	)
	if err != nil {
		t.Fatalf("RunStreaming() error = %v, want nil with non-zero exit result", err)
	}
	wantObservedBytes := streamingHelperOutputBytes()
	if observedStdout != wantObservedBytes || observedStderr != wantObservedBytes {
		t.Fatalf("observed output bytes = stdout %d/stderr %d, want %d each", observedStdout, observedStderr, wantObservedBytes)
	}
	if len(result.Stdout) > maxStreamingOutputBytes || len(result.Stderr) > maxStreamingOutputBytes {
		t.Fatalf("retained output bytes = stdout %d/stderr %d, want at most %d each", len(result.Stdout), len(result.Stderr), maxStreamingOutputBytes)
	}
	if !strings.Contains(string(result.Stdout), stdoutMarker) {
		t.Fatalf("retained stdout = %q, want terminal marker %q", result.Stdout, stdoutMarker)
	}
	if !strings.Contains(string(result.Stderr), stderrMarker) {
		t.Fatalf("retained stderr = %q, want terminal classification marker %q", result.Stderr, stderrMarker)
	}
	if result.ExitCode != 23 {
		t.Fatalf("ExitCode = %d, want 23", result.ExitCode)
	}
}

func streamingHelperOutputBytes() int {
	return maxStreamingOutputBytes*4 + len("streaming stdout complete\n")
}

func writeCommandHelperOutput(writer io.Writer, total int, marker string) {
	markerBytes := []byte(marker + "\n")
	remaining := total - len(markerBytes)
	chunk := []byte(strings.Repeat("x", 32<<10))
	for remaining > 0 {
		count := len(chunk)
		if count > remaining {
			count = remaining
		}
		if _, err := writer.Write(chunk[:count]); err != nil {
			fmt.Fprintf(os.Stderr, "write helper output: %v\n", err)
			os.Exit(2)
		}
		remaining -= count
	}
	if _, err := writer.Write(markerBytes); err != nil {
		fmt.Fprintf(os.Stderr, "write helper marker: %v\n", err)
		os.Exit(2)
	}
}

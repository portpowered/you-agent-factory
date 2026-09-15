package managedchild

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

func TestManagedStartPreservesStartErrorWithoutLaunchingAProcess(t *testing.T) {
	t.Parallel()

	startErr := errors.New("controlled start failure")
	_, err := startWithDependencies(context.Background(), Spec{Command: "controlled"}, startDependencies{
		newCommand:   exec.Command,
		configure:    func(*exec.Cmd) {},
		startCommand: func(*exec.Cmd) error { return startErr },
		attach: func(*exec.Cmd) (platformprocess.SubprocessTree, error) {
			t.Fatal("attach called after start failure")
			return platformprocess.SubprocessTree{}, nil
		},
		waitCommand: func(*exec.Cmd) error { t.Fatal("wait called after start failure"); return nil },
		terminate: func(*exec.Cmd, platformprocess.SubprocessTree) error {
			t.Fatal("terminate called after start failure")
			return nil
		},
		close: func(*exec.Cmd, platformprocess.SubprocessTree) {},
	})
	if !errors.Is(err, startErr) {
		t.Fatalf("start error = %v, want original identity", err)
	}
}

func TestManagedStartCleansAfterTreeAttachFailureAndJoinsOriginalErrors(t *testing.T) {
	t.Parallel()

	attachErr := errors.New("controlled attach failure")
	terminateErr := errors.New("controlled terminate failure")
	waitErr := errors.New("controlled wait failure")
	var terminateCalls, waitCalls atomic.Int32
	_, err := startWithDependencies(context.Background(), Spec{Command: "controlled"}, startDependencies{
		newCommand: exec.Command,
		configure:  func(*exec.Cmd) {},
		startCommand: func(cmd *exec.Cmd) error {
			cmd.Process = &os.Process{Pid: 123}
			return nil
		},
		attach: func(*exec.Cmd) (platformprocess.SubprocessTree, error) {
			return platformprocess.SubprocessTree{}, attachErr
		},
		waitCommand: func(*exec.Cmd) error {
			waitCalls.Add(1)
			return waitErr
		},
		terminate: func(*exec.Cmd, platformprocess.SubprocessTree) error {
			terminateCalls.Add(1)
			return terminateErr
		},
		close: func(*exec.Cmd, platformprocess.SubprocessTree) {},
	})
	for _, want := range []error{attachErr, terminateErr, waitErr} {
		if !errors.Is(err, want) {
			t.Fatalf("attach cleanup error = %v, want %v", err, want)
		}
	}
	if terminateCalls.Load() != 1 || waitCalls.Load() != 1 {
		t.Fatalf("attach cleanup calls = terminate %d/wait %d, want one each", terminateCalls.Load(), waitCalls.Load())
	}
}

func TestManagedOutputWriterCountsAndBoundsEveryStream(t *testing.T) {
	t.Parallel()

	capture := newStreamCapture(4)
	if _, err := capture.Write([]byte("abc")); err != nil {
		t.Fatalf("first output write: %v", err)
	}
	if _, err := capture.Write([]byte("defghi")); err != nil {
		t.Fatalf("second output write: %v", err)
	}

	got := capture.snapshot()
	digest := sha256.Sum256([]byte("abcdefghi"))
	if got.Bytes != 9 || got.SHA256 != hex.EncodeToString(digest[:]) || !got.Truncated {
		t.Fatalf("stream snapshot = %#v, want complete count/hash and truncation", got)
	}
	if string(got.Tail) != "fghi" || len(got.Tail) > 4 {
		t.Fatalf("stream tail = %q, want final four bytes", got.Tail)
	}

	below := newStreamCapture(outputTailLimit(DefaultOutputTailLimit + 1))
	_, _ = below.Write([]byte("complete"))
	if snapshot := below.snapshot(); snapshot.Truncated || string(snapshot.Tail) != "complete" {
		t.Fatalf("untruncated stream snapshot = %#v, want complete retained output", snapshot)
	}
}

func TestManagedProcessPublishesOneImmutableSnapshotToAllWaiters(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	waitErr := errors.New("controlled wait failure")
	stdout := newStreamCapture(8)
	stderr := newStreamCapture(8)
	_, _ = stdout.Write([]byte("stdout"))
	_, _ = stderr.Write([]byte("stderr"))
	process := &Process{
		stdout: stdout,
		stderr: stderr,
		waitFn: func() error {
			<-release
			return waitErr
		},
		closeFn: func() {},
		done:    make(chan struct{}),
	}
	go process.supervise()
	if _, ok := process.Snapshot(); ok {
		t.Fatal("pre-terminal Snapshot() = available, want unavailable")
	}

	waitResults := make(chan error, 8)
	for index := 0; index < cap(waitResults); index++ {
		go func() { waitResults <- process.Wait() }()
	}
	close(release)
	for index := 0; index < cap(waitResults); index++ {
		if got := <-waitResults; got != waitErr {
			t.Fatalf("Wait() error = %v, want the exact shared error %v", got, waitErr)
		}
	}

	first, ok := process.Snapshot()
	if !ok || first.ExitClass != ExitClassWaitFailed || first.Stdout.Bytes != 6 || first.Stderr.Bytes != 6 {
		t.Fatalf("terminal snapshot = (%#v, %v), want immutable wait-failure facts", first, ok)
	}
	first.Stdout.Tail[0] = 'X'
	second, ok := process.Snapshot()
	if !ok || string(second.Stdout.Tail) != "stdout" {
		t.Fatalf("Snapshot() did not deep-copy retained output: (%#v, %v)", second, ok)
	}
}

func TestManagedProcessStopIsIdempotentAndWaitsForTerminalization(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	var terminateCalls atomic.Int32
	process := &Process{
		stdout: newStreamCapture(DefaultOutputTailLimit),
		stderr: newStreamCapture(DefaultOutputTailLimit),
		waitFn: func() error {
			<-release
			return nil
		},
		terminateFn: func() error {
			terminateCalls.Add(1)
			close(release)
			return nil
		},
		closeFn: func() {},
		done:    make(chan struct{}),
	}
	go process.supervise()

	var wait sync.WaitGroup
	for index := 0; index < 16; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := process.Stop(context.Background()); err != nil {
				t.Errorf("concurrent Stop() = %v, want nil", err)
			}
		}()
	}
	wait.Wait()
	if terminateCalls.Load() != 1 {
		t.Fatalf("terminate calls = %d, want exactly once", terminateCalls.Load())
	}
	if err := process.Wait(); err != nil {
		t.Fatalf("Wait() after Stop() = %v, want nil", err)
	}
	if err := process.Stop(context.Background()); err != nil {
		t.Fatalf("repeated Stop() = %v, want nil", err)
	}
}

func TestManagedProcessStopHonorsWaitContextAfterCleanupBegins(t *testing.T) {
	t.Parallel()

	stopRelease := make(chan struct{})
	process := &Process{
		stdout: newStreamCapture(DefaultOutputTailLimit),
		stderr: newStreamCapture(DefaultOutputTailLimit),
		waitFn: func() error {
			<-stopRelease
			return nil
		},
		terminateFn: func() error { return nil },
		closeFn:     func() {},
		done:        make(chan struct{}),
	}
	go process.supervise()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := process.Stop(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop(canceled context) = %v, want context.Canceled", err)
	}
	close(stopRelease)
	if err := process.Wait(); err != nil {
		t.Fatalf("Wait() after canceled Stop() = %v, want nil", err)
	}
}

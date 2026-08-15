package process_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestACPServeCancellationPreservesContextCanceledIdentityThroughProcess
// drives the real customer boundary through root.BuildProcess and
// Process.Execute. The read-start signal makes cancellation deterministic:
// the ACP scanner is already blocked on stdin before the context is canceled,
// so the command's cancellation cleanup must both unblock the read and return
// the cancellation sentinel unchanged.
func TestACPServeCancellationPreservesContextCanceledIdentityThroughProcess(t *testing.T) {
	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)

	stdinReader, stdinWriter := io.Pipe()
	readStarted := newACPShutdownReadSignal(stdinReader)
	t.Cleanup(func() {
		_ = stdinWriter.Close()
		_ = stdinReader.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	var stdout, stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- process.Execute(root.Input{
			Args:             []string{"you", "serve", "acp"},
			Env:              append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir),
			Stdin:            readStarted,
			Stdout:           &stdout,
			Stderr:           &stderr,
			Context:          ctx,
			WorkingDirectory: workingDirectory,
		})
	}()

	// The timeout only guards against a genuine startup/read hang; the
	// read-start channel is the deterministic synchronization point.
	select {
	case <-readStarted.started:
	case <-time.After(5 * time.Second):
		t.Fatal("ACP server did not begin reading stdin before timeout")
	}
	cancel()

	select {
	case got := <-done:
		if got != context.Canceled {
			t.Fatalf("Process.Execute() error = %T %v, want context.Canceled by identity", got, got)
		}
		if got := stderr.String(); got != "Error: context canceled\n" {
			t.Fatalf("canceled ACP stderr = %q, want exact cancellation diagnostic", got)
		}
		if stdout.Len() != 0 {
			t.Fatalf("canceled ACP stdout = %q, want no protocol output", stdout.String())
		}
	// The timeout only guards against a regression that fails to unwind the
	// blocked stdin read after cancellation.
	case <-time.After(5 * time.Second):
		t.Fatal("Process.Execute() did not return after ACP cancellation")
	}
}

type acpShutdownReadSignal struct {
	reader  io.Reader
	closer  io.Closer
	once    sync.Once
	started chan struct{}
}

func newACPShutdownReadSignal(stream io.ReadCloser) *acpShutdownReadSignal {
	return &acpShutdownReadSignal{
		reader:  stream,
		closer:  stream,
		started: make(chan struct{}),
	}
}

func (r *acpShutdownReadSignal) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.reader.Read(p)
}

func (r *acpShutdownReadSignal) Close() error {
	return r.closer.Close()
}

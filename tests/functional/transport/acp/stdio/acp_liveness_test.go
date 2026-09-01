package stdio_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"sync"
	"testing"
	"time"
)

// These are failure ceilings for the package-owned protocol and lifecycle
// edges. They are deliberately separate from the Go test timeout: a stalled
// frame, signal, write, or command completion must identify its own stage and
// fail well before the package-level process is killed without context.
const (
	acpFrameReadCeiling         = 10 * time.Second
	acpFrameWriteCeiling        = 10 * time.Second
	acpCommandCompletionCeiling = 10 * time.Second
	acpControlSignalCeiling     = 10 * time.Second
	acpReadAbortCeiling         = time.Second
)

var errACPWaitCeiling = errors.New("ACP stdio wait ceiling elapsed")

type acpFrameReadResult struct {
	line  string
	bytes []byte
	err   error
}

// acpFrameReader owns the test side of one process stdout pipe. A read is
// performed in a short-lived goroutine so closing the OS pipe can interrupt
// the same blocking read that was previously parked until the package
// timeout. The result channel is buffered, allowing the read goroutine to
// report after a diagnostic has already caused the test to stop.
type acpFrameReader struct {
	file     *os.File
	reader   *bufio.Reader
	selector string

	closeOnce sync.Once
	closeErr  error
}

func newACPFrameReader(file *os.File, selector string) *acpFrameReader {
	return &acpFrameReader{
		file:     file,
		reader:   bufio.NewReader(file),
		selector: selector,
	}
}

func (r *acpFrameReader) close() error {
	if r == nil || r.file == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.closeErr = r.file.Close()
	})
	return r.closeErr
}

func (r *acpFrameReader) readLine(t *testing.T, stage string) string {
	t.Helper()
	result := make(chan acpFrameReadResult, 1)
	go func() {
		line, err := r.reader.ReadString('\n')
		result <- acpFrameReadResult{line: line, err: err}
	}()

	timer := time.NewTimer(acpFrameReadCeiling)
	defer timer.Stop()
	select {
	case outcome := <-result:
		if outcome.err != nil {
			failACPStdioWait(t, r.selector, stage, "bufio.Reader.ReadString('\\n')", "newline-terminated ACP frame on process stdout", outcome.err)
		}
		return outcome.line
	case <-timer.C:
		closeErr := r.close()
		waitForACPReadAbort(t, result, r.selector, stage, "bufio.Reader.ReadString('\\n')", "newline-terminated ACP frame on process stdout", closeErr)
		return ""
	}
}

func (r *acpFrameReader) readAll(t *testing.T, stage string) []byte {
	t.Helper()
	result := make(chan acpFrameReadResult, 1)
	go func() {
		payload, err := io.ReadAll(r.reader)
		result <- acpFrameReadResult{bytes: payload, err: err}
	}()

	timer := time.NewTimer(acpFrameReadCeiling)
	defer timer.Stop()
	select {
	case outcome := <-result:
		if outcome.err != nil {
			failACPStdioWait(t, r.selector, stage, "io.ReadAll(bufio.Reader)", "EOF on process stdout after command completion", outcome.err)
		}
		return outcome.bytes
	case <-timer.C:
		closeErr := r.close()
		waitForACPReadAbort(t, result, r.selector, stage, "io.ReadAll(bufio.Reader)", "EOF on process stdout after command completion", closeErr)
		return nil
	}
}

func waitForACPReadAbort(
	t *testing.T,
	result <-chan acpFrameReadResult,
	selector, stage, operation, target string,
	closeErr error,
) {
	t.Helper()
	abortTimer := time.NewTimer(acpReadAbortCeiling)
	defer abortTimer.Stop()
	select {
	case <-result:
		failACPStdioWait(t, selector, stage, operation, target, fmt.Errorf("%w; stream close error=%v; read returned after close", errACPWaitCeiling, closeErr))
	case <-abortTimer.C:
		failACPStdioWait(t, selector, stage, operation, target, fmt.Errorf("%w; stream close error=%v; read goroutine did not return after close", errACPWaitCeiling, closeErr))
	}
}

// writeACPBytes applies the same package-local failure ceiling to the caller
// side of an OS pipe. Large malformed-frame fixtures otherwise could park in
// Write before the response reader gets a chance to report the actual stage.
func writeACPBytes(t *testing.T, writer io.Writer, payload []byte, stage string) {
	t.Helper()
	result := make(chan error, 1)
	go func() {
		written, err := writer.Write(payload)
		if err == nil && written != len(payload) {
			err = io.ErrShortWrite
		}
		result <- err
	}()

	timer := time.NewTimer(acpFrameWriteCeiling)
	defer timer.Stop()
	select {
	case err := <-result:
		if err != nil {
			failACPStdioWait(t, t.Name(), stage, "io.Writer.Write", "complete ACP request frame on process stdin", err)
		}
	case <-timer.C:
		var closeErr error
		if closer, ok := writer.(io.Closer); ok {
			closeErr = closer.Close()
		}
		abortTimer := time.NewTimer(acpReadAbortCeiling)
		defer abortTimer.Stop()
		select {
		case <-result:
			failACPStdioWait(t, t.Name(), stage, "io.Writer.Write", "complete ACP request frame on process stdin", fmt.Errorf("%w; stream close error=%v; write returned after close", errACPWaitCeiling, closeErr))
		case <-abortTimer.C:
			failACPStdioWait(t, t.Name(), stage, "io.Writer.Write", "complete ACP request frame on process stdin", fmt.Errorf("%w; stream close error=%v; write goroutine did not return after close", errACPWaitCeiling, closeErr))
		}
	}
}

func waitForACPSignal[T any](t *testing.T, selector, stage, target string, signal <-chan T) T {
	t.Helper()
	timer := time.NewTimer(acpControlSignalCeiling)
	defer timer.Stop()
	select {
	case value := <-signal:
		return value
	case <-timer.C:
		failACPStdioWait(t, selector, stage, "channel receive", target, errACPWaitCeiling)
		var zero T
		return zero
	}
}

func waitForACPCommand(t *testing.T, done <-chan struct{}, stage string, closeOnTimeout func()) {
	t.Helper()
	timer := time.NewTimer(acpCommandCompletionCeiling)
	defer timer.Stop()
	select {
	case <-done:
		return
	case <-timer.C:
		if closeOnTimeout != nil {
			closeOnTimeout()
		}
		failACPStdioWait(t, t.Name(), stage, "support.ProcessCommand.Done channel receive", "Process.Execute goroutine completion after ACP stdin EOF", errACPWaitCeiling)
	}
}

func failACPStdioWait(t testing.TB, selector, stage, operation, target string, err error) {
	t.Helper()
	dump := acpGoroutineDump()
	t.Fatalf("ACP stdio bounded wait failed: selector=%q stage=%q source_operation=%q wait_target=%q error=%v\n--- goroutine dump (debug=2; exact parked goroutine is identified by its stack) ---\n%s", selector, stage, operation, target, err, dump)
}

func acpGoroutineDump() string {
	var dump bytes.Buffer
	if profile := pprof.Lookup("goroutine"); profile != nil {
		if err := profile.WriteTo(&dump, 2); err != nil {
			return fmt.Sprintf("goroutine profile unavailable: %v", err)
		}
	}
	return dump.String()
}

func readRPCFrame(t *testing.T, reader *acpFrameReader, stage string) rpcFrame {
	t.Helper()
	line := reader.readLine(t, stage)
	assertLineIsProtocolFrame(t, line)
	var frame rpcFrame
	if err := json.Unmarshal([]byte(line), &frame); err != nil {
		t.Fatalf("unmarshal RPC line: %v", err)
	}
	return frame
}

func closeACPStreamFiles(files ...*os.File) {
	for _, file := range files {
		if file != nil {
			_ = file.Close()
		}
	}
}

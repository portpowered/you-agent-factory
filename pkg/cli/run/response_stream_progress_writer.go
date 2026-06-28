package run

import (
	"io"
	"sync"
	"time"
)

const (
	defaultResponseStreamProgressQueueCapacity = 64
	responseStreamProgressDrainTimeout         = 250 * time.Millisecond
)

// responseStreamProgressWriter decouples internal stream consumption from
// terminal stdout writes so a slow or blocked consumer does not stall provider
// dispatch or invocation completion indefinitely.
type responseStreamProgressWriter struct {
	mu           sync.Mutex
	output       io.Writer
	queue        chan []byte
	wg           sync.WaitGroup
	closed       bool
	droppedLines int
}

func newResponseStreamProgressWriter(output io.Writer) *responseStreamProgressWriter {
	if output == nil {
		panic("response stream progress writer output is nil")
	}
	writer := &responseStreamProgressWriter{
		output: output,
		queue:  make(chan []byte, defaultResponseStreamProgressQueueCapacity),
	}
	writer.wg.Add(1)
	go writer.run()
	return writer
}

func (w *responseStreamProgressWriter) enqueue(payload []byte) bool {
	if w == nil {
		return false
	}
	line := appendPayloadLine(payload)

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return false
	}
	w.mu.Unlock()

	select {
	case w.queue <- line:
		return true
	default:
		w.mu.Lock()
		w.droppedLines++
		w.mu.Unlock()
		return false
	}
}

func (w *responseStreamProgressWriter) enqueueNotice(payload []byte) {
	if w == nil {
		return
	}
	line := appendPayloadLine(payload)

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		_, _ = w.output.Write(line)
		return
	}
	w.mu.Unlock()

	select {
	case w.queue <- line:
		return
	default:
	}

	select {
	case w.queue <- line:
	case <-time.After(50 * time.Millisecond):
		go func() {
			_, _ = w.output.Write(line)
		}()
	}
}

func (w *responseStreamProgressWriter) droppedProgressLines() int {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.droppedLines
}

func (w *responseStreamProgressWriter) stopAndDrain() {
	if w == nil {
		return
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		waitProgressWriter(&w.wg, responseStreamProgressDrainTimeout)
		return
	}
	w.closed = true
	w.mu.Unlock()
	close(w.queue)
	waitProgressWriter(&w.wg, responseStreamProgressDrainTimeout)
}

func waitProgressWriter(wg *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func (w *responseStreamProgressWriter) run() {
	defer w.wg.Done()
	for line := range w.queue {
		_, _ = w.output.Write(line)
	}
}

func appendPayloadLine(payload []byte) []byte {
	line := make([]byte, len(payload)+1)
	copy(line, payload)
	line[len(payload)] = '\n'
	return line
}

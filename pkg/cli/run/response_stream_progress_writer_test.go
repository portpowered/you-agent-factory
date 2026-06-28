package run

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type gatedResponseStreamWriter struct {
	mu                   sync.Mutex
	blocked              bool
	buf                  strings.Builder
	blockedWriteAttempts atomic.Int64
}

func (w *gatedResponseStreamWriter) block() {
	w.mu.Lock()
	w.blocked = true
	w.mu.Unlock()
}

func (w *gatedResponseStreamWriter) release() {
	w.mu.Lock()
	w.blocked = false
	w.mu.Unlock()
}

func (w *gatedResponseStreamWriter) Write(p []byte) (int, error) {
	for {
		w.mu.Lock()
		blocked := w.blocked
		w.mu.Unlock()
		if !blocked {
			return w.buf.Write(p)
		}
		w.blockedWriteAttempts.Add(1)
		time.Sleep(1 * time.Millisecond)
	}
}

func (w *gatedResponseStreamWriter) String() string {
	return w.buf.String()
}

func (w *gatedResponseStreamWriter) blockedWriteAttemptsCount() int64 {
	if w == nil {
		return 0
	}
	return w.blockedWriteAttempts.Load()
}

func TestResponseStreamProgressWriter_EnqueueDoesNotBlockWhenOutputSlow(t *testing.T) {
	t.Parallel()

	output := &gatedResponseStreamWriter{}
	output.block()
	writer := newResponseStreamProgressWriter(output)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < defaultResponseStreamProgressQueueCapacity+8; i++ {
			writer.enqueue([]byte("line"))
		}
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("enqueue blocked while terminal output was slow")
	}

	if got := writer.droppedProgressLines(); got == 0 {
		t.Fatalf("dropped lines = 0, want backlog drops when queue is full")
	}

	output.release()
	writer.stopAndDrain()
}

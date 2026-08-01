package service

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type gatedPresentationWriter struct {
	gate     chan struct{}
	attempts atomic.Int64
	mu       sync.Mutex
	buffer   bytes.Buffer
}

func newGatedPresentationWriter() *gatedPresentationWriter {
	return &gatedPresentationWriter{gate: make(chan struct{})}
}

func (w *gatedPresentationWriter) Write(payload []byte) (int, error) {
	w.attempts.Add(1)
	<-w.gate
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.Write(payload)
}

func (w *gatedPresentationWriter) release() { close(w.gate) }

func waitForPresentationWriteAttempt(t *testing.T, writer *gatedPresentationWriter) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if writer.attempts.Load() > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for presentation write")
}

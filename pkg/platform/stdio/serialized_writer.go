package stdio

import (
	"io"
	"sync"
)

// SerializeWrites wraps out so concurrent Write calls from independent
// goroutines are serialized into complete, non-interleaved writes: each
// call to the returned writer's Write forwards to out exactly once, under
// lock, so two callers can never tear or interleave each other's bytes. It
// adds no buffering or framing of its own.
func SerializeWrites(out io.Writer) io.Writer {
	return &serializedWriter{out: out}
}

type serializedWriter struct {
	mu  sync.Mutex
	out io.Writer
}

func (w *serializedWriter) Write(payload []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out.Write(payload)
}

package process

import (
	"io"
	"sync"
)

const (
	OutputStreamStdout = "stdout"
	OutputStreamStderr = "stderr"

	// maxStreamingOutputBytes bounds the retained result for each stream. The
	// observer still receives every chunk, so protocol decoders can enforce
	// their own source limits without the subprocess result retaining the whole
	// rollout. The retained tail keeps terminal stderr classifications useful.
	maxStreamingOutputBytes         = 64 << 10
	streamingOutputTruncationMarker = "[streaming output truncated; retained tail]\n"
)

// OutputChunkObserver receives incremental stdout or stderr bytes while a
// subprocess is still running.
type OutputChunkObserver func(stream string, chunk []byte)

type observedBuffer struct {
	mu        sync.Mutex
	stream    string
	observer  OutputChunkObserver
	buf       []byte
	maxBytes  int
	truncated bool
}

func (b *observedBuffer) Write(p []byte) (int, error) {
	if b.observer != nil && len(p) > 0 {
		chunk := append([]byte(nil), p...)
		b.observer(b.stream, chunk)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.retain(p)
	return len(p), nil
}

func (b *observedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf...)
}

func (b *observedBuffer) retain(p []byte) {
	if len(p) == 0 {
		return
	}
	if b.maxBytes <= 0 {
		b.buf = append(b.buf, p...)
		return
	}
	if len(b.buf)+len(p) <= b.maxBytes {
		b.buf = append(b.buf, p...)
		return
	}

	wasTruncated := b.truncated
	b.truncated = true
	tailLimit := b.maxBytes - len(streamingOutputTruncationMarker)
	if tailLimit <= 0 {
		b.buf = append(b.buf[:0], streamingOutputTruncationMarker[:b.maxBytes]...)
		return
	}
	tail := p
	if len(tail) > tailLimit {
		tail = tail[len(tail)-tailLimit:]
	}
	previous := b.buf
	if wasTruncated && len(previous) >= len(streamingOutputTruncationMarker) {
		previous = previous[len(streamingOutputTruncationMarker):]
	}
	remaining := tailLimit - len(tail)
	if len(previous) > remaining {
		previous = previous[len(previous)-remaining:]
	}
	retained := make([]byte, 0, b.maxBytes)
	retained = append(retained, streamingOutputTruncationMarker...)
	retained = append(retained, previous...)
	retained = append(retained, tail...)
	b.buf = retained
}

var _ io.Writer = (*observedBuffer)(nil)

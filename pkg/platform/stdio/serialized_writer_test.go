package stdio

import (
	"bytes"
	"sync"
	"testing"
)

// blockingWriter lets a test hold one Write call open until release is
// closed, so a second concurrent Write attempt can be proven to wait for it
// rather than interleaving.
type blockingWriter struct {
	mu      sync.Mutex
	out     bytes.Buffer
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (w *blockingWriter) Write(payload []byte) (int, error) {
	w.once.Do(func() {
		if w.entered != nil {
			close(w.entered)
		}
		if w.release != nil {
			<-w.release
		}
	})
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out.Write(payload)
}

func TestSerializeWritesForwardsEveryByte(t *testing.T) {
	var out bytes.Buffer
	writer := SerializeWrites(&out)
	if _, err := writer.Write([]byte("hello ")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := writer.Write([]byte("world")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := out.String(); got != "hello world" {
		t.Fatalf("out = %q, want %q", got, "hello world")
	}
}

func TestSerializeWritesNeverInterleavesConcurrentWrites(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	inner := &blockingWriter{entered: entered, release: release}
	writer := SerializeWrites(inner)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		if _, err := writer.Write([]byte("AAAA")); err != nil {
			t.Errorf("first Write() error = %v", err)
		}
	}()

	<-entered

	secondStarted := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		close(secondStarted)
		if _, err := writer.Write([]byte("BBBB")); err != nil {
			t.Errorf("second Write() error = %v", err)
		}
		close(secondDone)
	}()
	<-secondStarted

	select {
	case <-secondDone:
		t.Fatal("second Write() returned before the first, still-blocked Write() released -- writes were not serialized")
	default:
	}

	close(release)
	<-firstDone
	<-secondDone

	got := inner.out.String()
	if got != "AAAABBBB" && got != "BBBBAAAA" {
		t.Fatalf("out = %q, want the two writes concatenated whole, in either order, never interleaved", got)
	}
}

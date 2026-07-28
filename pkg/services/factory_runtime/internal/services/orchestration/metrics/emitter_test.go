package metrics

import (
	"context"
	"os"
	"testing"
)

func TestNoopEmitterDoesNotPanicOrWriteFiles(t *testing.T) {
	t.Chdir(t.TempDir())

	emitter := NoopEmitter{}
	fields := Fields{
		DispatchID:  "dispatch-1",
		WorkID:      "work-1",
		TraceID:     "trace-1",
		Workstation: "review",
		WorkerType:  "agent",
		Provider:    "codex",
		Outcome:     "ACCEPTED",
		Reason:      "test",
	}

	if err := emitter.Counter(context.Background(), "dispatch.started", 1, fields); err != nil {
		t.Fatalf("Counter returned error: %v", err)
	}
	if err := emitter.Gauge(context.Background(), "queue.depth", 3, fields); err != nil {
		t.Fatalf("Gauge returned error: %v", err)
	}
	if err := emitter.Sample(context.Background(), "dispatch.duration", 12.5, "ms", fields); err != nil {
		t.Fatalf("Sample returned error: %v", err)
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read test working directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("NoopEmitter wrote files: %v", entries)
	}
}

func TestEnsureEmitterDefaultsToNoop(t *testing.T) {
	emitter := EnsureEmitter(nil)
	if emitter == nil {
		t.Fatal("EnsureEmitter(nil) returned nil")
	}
	if err := emitter.Counter(context.Background(), "runtime.started", 1, Fields{}); err != nil {
		t.Fatalf("default emitter Counter returned error: %v", err)
	}
}

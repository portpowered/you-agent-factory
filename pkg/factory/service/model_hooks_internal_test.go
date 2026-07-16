package service

import (
	"context"
	"testing"
	"time"
)

func TestLocalModelHooksRecordResourceAndLoadLifecycle(t *testing.T) {
	t.Parallel()

	trace := &modelExecutionEventTrace{}
	ctx := context.WithValue(context.Background(), modelExecutionEventTraceKey{}, trace)
	start := time.Unix(100, 0)
	hooks := LocalModelHooks()
	hooks.MarkResourceWaitStarted(ctx, start)
	hooks.MarkResourceWaitFinished(ctx, start.Add(25*time.Millisecond), true)
	hooks.MarkLoadRequested(ctx, start)
	hooks.MarkLoadFinished(ctx, start.Add(40*time.Millisecond))
	hooks.MarkLoadReused(ctx)

	if trace.resourceWaitMillis != 25 || !trace.resourceAcquired || trace.loadMillis != 40 || !trace.loadRequested || !trace.loadReused {
		t.Fatalf("model execution trace = %+v, want recorded wait/load lifecycle", trace)
	}
	// Nil contexts are an explicit no-op boundary for model hooks.
	hooks.MarkResourceWaitStarted(nil, start)
	hooks.MarkResourceWaitFinished(nil, start, false)
	hooks.MarkLoadRequested(nil, start)
	hooks.MarkLoadFinished(nil, start)
	hooks.MarkLoadReused(nil)
}

package engine

import (
	"context"
	"errors"
	"testing"
)

func TestNormalizedSubmissionRejectsCanceledContextBeforeEngineState(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	engine := &FactoryEngine{}
	if _, err := engine.submitNormalizedWorkRequest(ctx, "request", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled submission = %v", err)
	}
}

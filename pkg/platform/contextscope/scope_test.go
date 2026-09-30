package contextscope

import (
	"context"
	"testing"
)

func TestScopeStopsChildWithoutCancelingParent(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()

	scope := New(parent)
	if err := scope.Context().Err(); err != nil {
		t.Fatalf("new scope context = %v, want active", err)
	}
	scope.Stop()
	if got := scope.Context().Err(); got != context.Canceled {
		t.Fatalf("stopped scope context = %v, want canceled", got)
	}
	if err := parent.Err(); err != nil {
		t.Fatalf("parent context = %v, want active", err)
	}
}

func TestScopeFollowsParentCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	scope := New(parent)
	cancelParent()
	if got := scope.Context().Err(); got != context.Canceled {
		t.Fatalf("scope context after parent cancellation = %v, want canceled", got)
	}
	scope.Stop()
}

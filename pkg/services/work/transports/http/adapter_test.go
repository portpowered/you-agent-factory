package http

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestAdapter_BindsWorkRootViaFakeRootSeam(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &rootFake{
		listWork: func(
			_ context.Context,
			sessionID string,
			options work.ListOptions,
		) (work.ListResult, error) {
			invoked = true
			if sessionID != "session-1" {
				t.Fatalf("sessionID = %q, want session-1", sessionID)
			}
			if options.MaxResults != 10 {
				t.Fatalf("ListOptions = %#v, want maxResults 10", options)
			}
			return work.ListResult{}, work.ErrWorkNotFound
		},
	}

	adapter := NewAdapter(fake)
	if adapter.Root() != fake {
		t.Fatal("adapter must expose the injected Work root")
	}

	_, err := adapter.invokeListWork(context.Background(), "session-1", work.ListOptions{MaxResults: 10})
	if !invoked {
		t.Fatal("adapter-owned operation did not invoke the injected Work root")
	}
	if !errors.Is(err, work.ErrWorkNotFound) {
		t.Fatalf("invokeListWork error = %v, want ErrWorkNotFound", err)
	}
}

func TestAdapterMissingRootReportsOperationError(t *testing.T) {
	t.Parallel()
	_, err := NewAdapter(nil).invokeListWork(context.Background(), "session-1", work.ListOptions{})
	if err == nil || err.Error() != "work service is required" {
		t.Fatalf("missing root error = %v", err)
	}
}

func TestAdapterAdmissionBindingUsesCompleteRoot(t *testing.T) {
	t.Parallel()
	original := &rootFake{listWork: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
		return work.ListResult{}, work.ErrWorkNotFound
	}}
	replacement := &rootFake{listWork: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
		return work.ListResult{}, context.Canceled
	}}
	adapter := NewAdapter(original)
	bound := adapter.WithAdmissionService(replacement)
	_, err := bound.invokeListWork(context.Background(), "session-1", work.ListOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("bound list error = %v", err)
	}
	_, err = adapter.invokeListWork(context.Background(), "session-1", work.ListOptions{})
	if !errors.Is(err, work.ErrWorkNotFound) {
		t.Fatalf("original list error = %v", err)
	}
}

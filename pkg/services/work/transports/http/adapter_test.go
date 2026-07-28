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

func TestNewAdapter_RejectsNilRoot(t *testing.T) {
	t.Parallel()

	if NewAdapter(nil) != nil {
		t.Fatal("NewAdapter(nil) must return nil")
	}
}

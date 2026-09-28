package http

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestAdapter_BindsRecordingsRootViaFakeRootSeam(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &rootFake{
		subscribeFrom: func(
			_ context.Context,
			request recordings.SubscribeRequest,
		) (recordings.SubscribeResult, error) {
			invoked = true
			if request.Scope.FactorySessionID != "session-1" {
				t.Fatalf("SubscribeRequest scope = %#v, want factory session session-1", request.Scope)
			}
			return recordings.SubscribeResult{}, recordings.ErrReconnectCursorNotFound
		},
	}

	adapter := NewAdapter(fake)
	if adapter.Root() != fake {
		t.Fatal("adapter must expose the injected Recordings root")
	}

	_, err := adapter.invokeSubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if !invoked {
		t.Fatal("adapter-owned operation did not invoke the injected Recordings root")
	}
	if !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("invokeSubscribeFrom error = %v, want ErrReconnectCursorNotFound", err)
	}
}

func TestNewAdapter_RejectsNilRoot(t *testing.T) {
	t.Parallel()

	if NewAdapter(nil) != nil {
		t.Fatal("NewAdapter(nil) must return nil")
	}
}

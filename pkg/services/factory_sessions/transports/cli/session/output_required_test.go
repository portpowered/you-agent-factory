package session

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type testHTTPClock struct{}

func (testHTTPClock) Now() time.Time { return time.Unix(1, 0) }

func testHTTPProtocol(t *testing.T) clihttp.Protocol {
	t.Helper()
	protocol, err := clihttp.NewProtocol(&http.Client{}, testHTTPClock{})
	if err != nil {
		t.Fatalf("build test HTTP protocol: %v", err)
	}
	return protocol
}

func TestSessionCommands_RequireCallerOwnedOutput(t *testing.T) {
	t.Parallel()

	tests := map[string]func() error{
		"create": func() error { return NewCreate(testHTTPProtocol(t))(CreateConfig{Dir: "."}) },
		"delete": func() error { return NewDelete(testHTTPProtocol(t))(DeleteConfig{SessionID: "session-1"}) },
		"dispatches": func() error {
			return NewDispatches(testHTTPProtocol(t))(DispatchesConfig{Context: context.Background()})
		},
		"pause": func() error {
			return NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background()})
		},
		"resume": func() error {
			return NewResume(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background()})
		},
		"list": func() error {
			return NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background()})
		},
		"show": func() error { return NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background()}) },
	}
	for name, run := range tests {
		name, run := name, run
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := run(); err == nil || err.Error() != "output writer is required" {
				t.Fatalf("error = %v, want output writer is required", err)
			}
		})
	}
}

func TestBindServiceDelegatesToInjectedOperations(t *testing.T) {
	t.Parallel()

	calls := 0
	service := Bind(Operations{
		List: func(ListConfig) error {
			calls++
			return nil
		},
		Show:           func(ShowConfig) error { return nil },
		Pause:          func(LifecycleControlConfig) error { return nil },
		Resume:         func(LifecycleControlConfig) error { return nil },
		ListDispatches: func(DispatchesConfig) error { return nil },
		Create:         func(CreateConfig) error { return nil },
		Delete:         func(DeleteConfig) error { return nil },
	})
	if err := service.List(ListConfig{Context: context.Background()}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestBindServiceRequiresInjectedOperations(t *testing.T) {
	t.Parallel()

	service := Bind(Operations{})
	if err := service.List(ListConfig{Context: context.Background()}); err == nil {
		t.Fatal("List() error = nil, want required-edge failure")
	}
}

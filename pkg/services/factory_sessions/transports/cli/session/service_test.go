package session_test

import (
	"context"
	"testing"

	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
)

func TestBindServiceDelegatesToInjectedOperations(t *testing.T) {
	t.Parallel()

	calls := 0
	service := sessioncli.Bind(sessioncli.Operations{
		List: func(sessioncli.ListConfig) error {
			calls++
			return nil
		},
		Show:           func(sessioncli.ShowConfig) error { return nil },
		Pause:          func(sessioncli.LifecycleControlConfig) error { return nil },
		Resume:         func(sessioncli.LifecycleControlConfig) error { return nil },
		ListDispatches: func(sessioncli.DispatchesConfig) error { return nil },
		Create:         func(sessioncli.CreateConfig) error { return nil },
		Delete:         func(sessioncli.DeleteConfig) error { return nil },
	})
	if err := service.List(sessioncli.ListConfig{Context: context.Background()}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestBindServiceRequiresInjectedOperations(t *testing.T) {
	t.Parallel()

	service := sessioncli.Bind(sessioncli.Operations{})
	if err := service.List(sessioncli.ListConfig{Context: context.Background()}); err == nil {
		t.Fatal("List() error = nil, want required-edge failure")
	}
}

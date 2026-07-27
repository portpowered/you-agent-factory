package session

import (
	"context"
	"testing"
)

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

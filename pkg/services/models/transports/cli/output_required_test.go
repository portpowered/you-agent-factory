package cli

import (
	"context"
	"testing"
)

func TestModelCommands_RequireCallerOwnedOutput(t *testing.T) {
	t.Parallel()

	tests := map[string]func() error{
		"list": func() error {
			return New(testHTTPProtocol(t), testModelInvocationBuilder).List(ListConfig{Context: context.Background()})
		},
		"inspect": func() error {
			return New(testHTTPProtocol(t), testModelInvocationBuilder).Inspect(InspectConfig{Context: context.Background()})
		},
		"invoke": func() error { return invokeForTest(t, InvokeConfig{Context: context.Background()}) },
		"pull": func() error {
			return New(testHTTPProtocol(t), testModelInvocationBuilder).Pull(PullConfig{Context: context.Background()})
		},
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

package work

import (
	"bytes"
	"context"
	"testing"
)

func TestWorkCommands_RequireCallerOwnedOutput(t *testing.T) {
	t.Parallel()

	tests := map[string]func() error{
		"list": func() error {
			return NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background()})
		},
		"move": func() error { return NewMove(testHTTPProtocol(t))(MoveConfig{Context: context.Background()}) },
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

func TestShowRequiresCallerContext(t *testing.T) {
	t.Parallel()

	err := NewShow(testHTTPProtocol(t))(ShowConfig{Output: &bytes.Buffer{}})
	if err == nil || err.Error() != "context is required" {
		t.Fatalf("error = %v, want context is required", err)
	}
}

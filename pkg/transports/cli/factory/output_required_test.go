package factory

import (
	"context"
	"testing"
)

func TestFactoryCommands_RequireCallerOwnedOutput(t *testing.T) {
	t.Parallel()

	tests := map[string]func() error{
		"delete": func() error { return Delete(nil, DeleteConfig{}) },
		"list":   func() error { return List(nil, nil, ListConfig{}) },
		"query":  func() error { return NewQuery(testHTTPProtocol(t))(QueryConfig{Context: context.Background()}) },
		"replace current": func() error {
			return NewReplaceCurrent(testHTTPProtocol(t))(ReplaceCurrentConfig{Context: context.Background()})
		},
		"validate": func() error { return ValidateWithServices(ValidateConfig{Context: context.Background()}, nil, nil) },
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

package workflow

import "testing"

func TestWorkflowCommands_RequireCallerOwnedOutput(t *testing.T) {
	t.Parallel()

	tests := map[string]func() error{
		"preview":  func() error { return Preview(nil, PreviewConfig{}) },
		"validate": func() error { return Validate(nil, ValidateConfig{}) },
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

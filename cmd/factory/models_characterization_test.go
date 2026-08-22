package main

import "testing"

// TestModelsCLICharacterizationMapsValidationFailuresToFailureExit covers
// the real executable boundary. Process-level exit mapping is characterized,
// not endorsed: these current invalid Models inputs must remain failures.
func TestModelsCLICharacterizationMapsValidationFailuresToFailureExit(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing text", args: []string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--json"}},
		{name: "missing output", args: []string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--text", "hello"}},
		{name: "unsupported operation", args: []string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "INVALID"}},
		{name: "unknown flag", args: []string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--unknown"}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			args := append([]string{"you"}, testCase.args...)
			outcome := runEntrypoint(t, args...)
			if outcome.exitCode != exitFailure {
				t.Fatalf("runProcess(%v) exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", args, outcome.exitCode, exitFailure, outcome.stdout, outcome.stderr)
			}
		})
	}
}

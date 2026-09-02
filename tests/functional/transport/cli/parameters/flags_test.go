package parameters_test

import (
	"strings"
	"testing"
)

// TestCLIUnknownFlagFailsBeforeLifecycleStart proves an unknown or removed CLI
// flag is rejected with a stable customer diagnostic before command execution.
func TestCLIUnknownFlagFailsBeforeLifecycleStart(t *testing.T) {
	t.Parallel()
	inputs := parameterInputs(t, []string{
		"you", "init", "--legacy-scaffold", "legacy-factory",
	})

	executeErr := parameterProcesses.process.Execute(inputs.Input)
	if executeErr == nil || !strings.Contains(executeErr.Error(), "unknown flag: --legacy-scaffold") {
		t.Fatalf(
			"unknown init flag error = %v, want unknown flag: --legacy-scaffold; stdout=%q stderr=%q",
			executeErr,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
}

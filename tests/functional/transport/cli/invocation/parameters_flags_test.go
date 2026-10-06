package invocation_test

import (
	"strings"
	"testing"
)

// TestCLIUnknownFlagFailsBeforeLifecycleStart proves an unknown CLI
// flag is rejected with a stable customer diagnostic before command execution.
func TestCLIUnknownFlagFailsBeforeLifecycleStart(t *testing.T) {
	t.Parallel()
	inputs := parameterInputs(t, []string{
		"you", "init", "--does-not-exist", "legacy-factory",
	})

	executeErr := parameterProcessesForTest(t).process.Execute(inputs.Input)
	if executeErr == nil || !strings.Contains(executeErr.Error(), "unknown flag: --does-not-exist") {
		t.Fatalf(
			"unknown init flag error = %v, want unknown flag: --does-not-exist; stdout=%q stderr=%q",
			executeErr,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
}

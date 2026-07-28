package provider

import (
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
)

const (
	nativeScriptWrapHarnessProvider = "test-native-harness"

	claudeThrottleFailureMessage   = "Claude is temporarily unavailable due to rate or capacity limits."
	claudeTimeoutFailureMessage    = "Claude request timed out."
	claudeAuthFailureMessage       = "Claude authentication failed."
	claudeBadRequestFailureMessage = "Claude rejected the request as invalid."
	claudeConfigFailureMessage     = "Claude is not configured correctly."
	claudeFailureScanBytes         = 64 * 1024

	codexUnknownFailureMessage         = "Codex returned an unrecognized error."
	codexAuthFailureMessage            = "Codex authentication failed."
	codexThrottleFailureMessage        = "Codex is temporarily unavailable due to usage or capacity limits."
	codexServerFailureMessage          = "Codex encountered a temporary server error."
	codexTimeoutFailureMessage         = "Codex request timed out."
	codexBadRequestFailureMessage      = "Codex rejected the request as invalid."
	codexGPT56SolUpgradeMessage        = "Update the Codex CLI to run GPT-5.6-Sol."
	codexWindowsProcessFailureExitCode = 4294967295
)

func skipConductorRoutedNativeProviderTest(t *testing.T) {
	t.Helper()
	t.Skip("codex/claude native ScriptWrapProvider coverage replaced by conductor integrations")
}

func ParseClaudeProviderFailure(result CommandResult) ProviderFailureResult {
	return parseProviderExitFailure(string(modelprovider.ProviderClaude), result).failure
}

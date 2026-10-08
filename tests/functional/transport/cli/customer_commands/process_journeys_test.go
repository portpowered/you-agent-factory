package customer_commands_test

import "testing"

// TestCLIProcessJourneys owns its immutable fixture until all customer scenarios finish.
func TestCLIProcessJourneys(t *testing.T) {
	t.Parallel()
	initializeProcessFixture(t)
	t.Run("TestCLIStartupLoadCauseWithoutDebug", testCLIStartupLoadCauseWithoutDebug)
	t.Run("TestACPServeCancellationPreservesContextCanceledIdentityThroughProcess", testProcessACPServeCancellationPreservesContextCanceledIdentityThroughProcess)
	t.Run("TestCLIWorkerFailureExitCode", testProcessCLIWorkerFailureExitCode)
	t.Run("TestCLISuccessExitCode", testProcessCLISuccessExitCode)
	t.Run("TestCLIHelpListsPublicCommandFamilies", testProcessCLIHelpListsPublicCommandFamilies)
	t.Run("TestCLISubcommandHelpUsesStableUsageAndExitZero", testProcessCLISubcommandHelpUsesStableUsageAndExitZero)
	t.Run("TestCLIVersionWritesOneMachineReadableVersion", testProcessCLIVersionWritesOneMachineReadableVersion)
	t.Run("TestCLIGroupHelpRendersExactlyOnce", testProcessCLIGroupHelpRendersExactlyOnce)
	t.Run("TestCLIUnknownCommandWritesSafeCodedStderr", testProcessCLIUnknownCommandWritesSafeCodedStderr)
	t.Run("TestCLIValidationFailureReturnsErrorWithoutSuccessOutput", testProcessCLIValidationFailureReturnsErrorWithoutSuccessOutput)
	t.Run("TestCLISuccessWritesPrimaryResultOnlyToStdout", testProcessCLISuccessWritesPrimaryResultOnlyToStdout)
	t.Run("TestCLIFailureWritesDiagnosticToStderr", testProcessCLIFailureWritesDiagnosticToStderr)
	t.Run("TestCLIQuietModeSuppressesNonResultNoise", testProcessCLIQuietModeSuppressesNonResultNoise)
	t.Run("TestRunReadsPromptFromStdin", testProcessRunReadsPromptFromStdin)
	t.Run("TestCLIEmptyRequiredStdinFailsWithoutDispatch", testProcessCLIEmptyRequiredStdinFailsWithoutDispatch)
	t.Run("TestSubmitBatchReadsJSONFromStdin", testProcessSubmitBatchReadsJSONFromStdin)
	t.Run("TestCLIWorkWatchStreamAbortReturnsNonZeroExit", testProcessCLIWorkWatchStreamAbortReturnsNonZeroExit)
}

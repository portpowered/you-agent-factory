package providers

import (
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/pkg/workers/provider"
)

func TestInvocationErrorCompatibility_SupportedCorpusEntriesPreserveStableWorkFailureTypes(t *testing.T) {
	corpus, err := provider.LoadProviderErrorCorpus()
	if err != nil {
		t.Fatalf("LoadProviderErrorCorpus() error = %v", err)
	}

	for _, entry := range corpus.Entries() {
		if !entry.Supported {
			continue
		}
		t.Run(providerErrorCorpusEntryLabel(entry), func(t *testing.T) {
			providerErr := provider.NormalizeProviderExitFailure(string(entry.Provider), entry.CommandResult(), nil, nil)
			if providerErr.Type != entry.ExpectedType {
				t.Fatalf("ProviderError.Type = %q, want stable %q", providerErr.Type, entry.ExpectedType)
			}

			metadata := provider.WorkFailureMetadataFromError(providerErr)
			if metadata == nil || metadata.Type != entry.ExpectedType {
				t.Fatalf("WorkFailureMetadata.Type = %#v, want %q", metadata, entry.ExpectedType)
			}
			if metadata.Family != entry.ExpectedFamily {
				t.Fatalf("WorkFailureMetadata.Family = %q, want %q", metadata.Family, entry.ExpectedFamily)
			}

			detail := provider.SafeProviderFailureDetail(providerErr)
			if detail == nil || detail.Reason != entry.ExpectedType {
				t.Fatalf("FailureDetail.Reason = %#v, want %q", detail, entry.ExpectedType)
			}
		})
	}
}

func TestInvocationErrorCompatibility_CodexReportingPathsPreserveStableWorkFailureTypes(t *testing.T) {
	testCases := []struct {
		name          string
		streamMessage string
		exitStderr    string
		exitCode      int
		wantType      workerexecution.WorkFailureType
	}{
		{name: "auth", streamMessage: "unexpected status 401", exitStderr: "ERROR: unexpected status 401\n", exitCode: 1, wantType: workerexecution.WorkFailureTypeAuthFailure},
		{name: "invalid_request", streamMessage: "unexpected status 400", exitStderr: "ERROR: unexpected status 400\n", exitCode: 1, wantType: workerexecution.WorkFailureTypePermanentBadRequest},
		{name: "throttle", streamMessage: "unexpected status 429", exitStderr: "ERROR: unexpected status 429\n", exitCode: 1, wantType: workerexecution.WorkFailureTypeThrottled},
		{name: "capacity", streamMessage: "selected model is at capacity", exitStderr: "ERROR: selected model is at capacity\n", exitCode: 1, wantType: workerexecution.WorkFailureTypeThrottled},
		{name: "usage_limit", streamMessage: "you've hit your usage limit", exitStderr: "ERROR: you've hit your usage limit\n", exitCode: 1, wantType: workerexecution.WorkFailureTypeThrottled},
		{name: "timeout", streamMessage: "command timed out", exitStderr: "ERROR: command timed out\n", exitCode: 124, wantType: workerexecution.WorkFailureTypeTimeout},
		{name: "disconnect", streamMessage: "unexpected status 502", exitStderr: "ERROR: unexpected status 502\n", exitCode: 1, wantType: workerexecution.WorkFailureTypeInternalServerError},
		{name: "server", streamMessage: "unexpected status 503", exitStderr: "ERROR: unexpected status 503\n", exitCode: 1, wantType: workerexecution.WorkFailureTypeInternalServerError},
		{name: "malformed", streamMessage: "operation failed with private transcript details", exitStderr: `ERROR: {"type":"error","error":{"message":"private transcript"}}` + "\n", exitCode: 1, wantType: workerexecution.WorkFailureTypeUnknown},
		{name: "unknown", streamMessage: "cleanup detail that is not a recognized provider failure", exitStderr: "", exitCode: 17, wantType: workerexecution.WorkFailureTypeUnknown},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			streamResult, ok := provider.CodexStructuredStreamReportingOutcome(codexStructuredStreamStdout(tc.streamMessage))
			if !ok {
				t.Fatal("CodexStructuredStreamReportingOutcome() ok = false, want true")
			}
			exitResult := provider.CodexProcessExitReportingOutcome(codexProcessExitResult(tc.exitStderr, tc.exitCode))

			for _, label := range []string{"structured-stream", "process-exit"} {
				result := streamResult
				if label == "process-exit" {
					result = exitResult
				}
				providerErr := provider.NewProviderErrorFromResult(result, nil)
				if providerErr.Type != tc.wantType {
					t.Fatalf("%s ProviderError.Type = %q, want stable %q", label, providerErr.Type, tc.wantType)
				}
				detail := provider.SafeProviderFailureDetail(providerErr)
				if detail.Reason != tc.wantType {
					t.Fatalf("%s FailureDetail.Reason = %q, want %q", label, detail.Reason, tc.wantType)
				}
			}
		})
	}
}

func TestInvocationErrorCompatibility_CodexUnknownExitFallbackKeepsStableTypeWithAuditedMessage(t *testing.T) {
	providerErr := provider.NormalizeProviderExitFailure(
		string(modelprovider.Codex),
		provider.CommandResult{ExitCode: 7, Stderr: []byte("configured stderr")},
		nil,
		nil,
	)
	if providerErr.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("Type = %q, want stable %q", providerErr.Type, workerexecution.WorkFailureTypeUnknown)
	}
	if providerErr.Message != codexUnknownFailureMessage {
		t.Fatalf("Message = %q, want audited %q", providerErr.Message, codexUnknownFailureMessage)
	}
}

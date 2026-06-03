package provider

import (
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

func cursorInferenceFailureDiagnostics(
	cursorProvider bool,
	commandDiagnostics *interfaces.WorkDiagnostics,
	result CommandResult,
) *interfaces.WorkDiagnostics {
	if !cursorProvider {
		return commandDiagnostics
	}
	return withCursorCommandOutputExcerpts(commandDiagnostics, result.Stdout, result.Stderr)
}

func (p *ScriptWrapProvider) completeCursorInference(
	req interfaces.RunnerExecutionRequest,
	result CommandResult,
	commandDiagnostics *interfaces.WorkDiagnostics,
	logger logging.Logger,
) (interfaces.InferenceResponse, error) {
	parsed, parseErr := parseCursorInferenceResult(req.ModelProvider, result.Stdout)
	if parseErr != nil {
		logger.Error("inferencer: cursor JSON parse failed",
			cursorFailureLogFields(req, true, result,
				"error", parseErr.Message)...)
		failureDiagnostics := withCursorCommandOutputExcerpts(commandDiagnostics, result.Stdout, result.Stderr)
		return interfaces.InferenceResponse{}, newProviderErrorWithDiagnostics(
			parseErr.Type,
			parseErr.Message,
			parseErr.Cause,
			effectiveProviderSession(req, result),
			failureDiagnostics,
		)
	}
	diagnostics := withCursorResponseMetadata(commandDiagnostics, parsed.ResponseMetadata)
	logger.Debug("inference results:",
		workLogFields(req.Dispatch.Execution, "output", parsed.Content)...)
	logger.Info("inferencer: request completed",
		workLogFields(req.Dispatch.Execution,
			"dispatcher", string(req.ModelProvider),
			"output_len", len(parsed.Content),
			"session_id", parsed.ProviderSession.ID)...)
	return interfaces.InferenceResponse{
		Content:         parsed.Content,
		ProviderSession: parsed.ProviderSession,
		Diagnostics:     diagnostics,
	}, nil
}

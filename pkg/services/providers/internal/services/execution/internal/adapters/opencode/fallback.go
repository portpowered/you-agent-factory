package opencode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

const (
	maxUnsupportedRejectionBytes = 4096
	maxSafeVersionLength         = 48
	structuredModeDegradedCode   = "structured_mode_degraded"
)

var errMissingAuthoritativeResponse = errors.New("opencode stream did not contain an authoritative response")

type attemptOutcome struct {
	mode         Mode
	effectResult EffectResult
	effectErr    error
	flushErr     error
	finalErr     error
	decoder      *decoder
}

func (outcome attemptOutcome) terminalFailure() (execution.AttemptFailure, bool) {
	return collectFailure(outcome.decoder, outcome.effectErr, outcome.flushErr)
}

func (outcome attemptOutcome) successResult(diagnostic *providers.ExecuteProgress) (providers.ExecuteResult, error) {
	if failure, failed := outcome.terminalFailure(); failed {
		sessionRef := outcome.decoder.failureSessionRef()
		progress := outcome.decoder.progressFacts()
		failure = attachFailureObservation(failure, sessionRef, progress)
		return providers.ExecuteResult{SessionRef: sessionRef}, failure
	}
	content, session, finalErr := outcome.decoder.final()
	if finalErr != nil {
		return providers.ExecuteResult{}, execution.AttemptFailure{FinalParseError: finalErr}
	}
	progress := outcome.decoder.progressFacts()
	if diagnostic != nil {
		progress = append(progress, diagnostic.Clone())
	}
	return providers.ExecuteResult{
		Content:    content,
		SessionRef: session,
		Diagnostics: &providers.ExecuteDiagnostics{
			DurationMillis: outcome.effectResult.DurationMillis,
			Progress:       progress,
			Metadata:       cloneMetadata(outcome.effectResult.Metadata),
		},
	}, nil
}

func plansStructuredFallback(
	ctx context.Context,
	request providers.ExecuteRequest,
	outcome attemptOutcome,
	requireStructuredStream bool,
) bool {
	if err := ctx.Err(); err != nil || outcome.mode != ModeStructured {
		return false
	}
	if requireStructuredStream || requiresStructuredStream(request) {
		return false
	}
	if outcome.decoder.declaredFailure != nil ||
		outcome.decoder.decodeErr != nil ||
		outcome.flushErr != nil ||
		outcome.effectResult.Command.RunError != nil {
		return false
	}
	if isLifecycleFailure(outcome.effectErr) {
		return false
	}
	if hasWorkProgress(outcome.decoder) {
		return false
	}
	command := outcome.effectResult.Command
	if command.ExitCode == 0 || len(bytes.TrimSpace(command.Stdout)) != 0 {
		return false
	}
	stderrBytes := len(command.Stderr)
	if stderrBytes == 0 || stderrBytes > maxUnsupportedRejectionBytes {
		return false
	}
	if outcome.finalErr != nil && !errors.Is(outcome.finalErr, errMissingAuthoritativeResponse) {
		return false
	}
	message := strings.ToLower(strings.Join(strings.Fields(string(command.Stderr)), " "))
	return positiveUnsupportedFormatSignal(message)
}

func requiresStructuredStream(request providers.ExecuteRequest) bool {
	if value, ok := request.EnvVars["providers_require_structured_stream"]; ok {
		return strings.TrimSpace(value) == "true"
	}
	return false
}

func hasWorkProgress(decoder *decoder) bool {
	for _, fact := range decoder.progressFacts() {
		if fact.Phase != "diagnostic" {
			return true
		}
	}
	return false
}

func isLifecycleFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var lifecycle execution.AttemptFailure
	if errors.As(err, &lifecycle) {
		if lifecycle.Declared != nil {
			switch lifecycle.Declared.Kind {
			case providers.ExecuteFailureKindCanceled, providers.ExecuteFailureKindTimeout:
				return true
			}
		}
	}
	var declared providers.ExecuteFailure
	if errors.As(err, &declared) {
		switch declared.Kind {
		case providers.ExecuteFailureKindCanceled, providers.ExecuteFailureKindTimeout:
			return true
		}
	}
	return false
}

func positiveUnsupportedFormatSignal(message string) bool {
	if !strings.Contains(message, "--format") {
		return false
	}
	for _, signal := range []string{"unknown option", "unknown flag", "unrecognized option", "unexpected argument"} {
		if strings.Contains(message, signal) {
			return true
		}
	}
	return strings.Contains(message, "json") &&
		(strings.Contains(message, "invalid value") ||
			strings.Contains(message, "not supported") ||
			strings.Contains(message, "unsupported"))
}

func degradationDiagnostic(version string) providers.ExecuteProgress {
	return providers.ExecuteProgress{
		Phase:  "diagnostic",
		Detail: degradationDiagnosticMessage(version),
		Metadata: map[string]string{
			"code": structuredModeDegradedCode,
		},
	}
}

func degradationDiagnosticMessage(version string) string {
	return fmt.Sprintf(
		"OpenCode selected final_only after unsupported_format rejection (version %s).",
		safeVersionContext(version),
	)
}

func safeVersionContext(version string) string {
	for _, field := range strings.Fields(version) {
		if len(field) > maxSafeVersionLength || !strings.ContainsFunc(field, unicode.IsDigit) {
			continue
		}
		valid := true
		for _, char := range field {
			if !unicode.IsLetter(char) && !unicode.IsDigit(char) && !strings.ContainsRune("._+-", char) {
				valid = false
				break
			}
		}
		if valid {
			return field
		}
	}
	return "unknown"
}

const failureStageDecode = "decode"

func attachFailureObservation(
	failure execution.AttemptFailure,
	sessionRef *providers.SessionRef,
	progress []providers.ExecuteProgress,
) execution.AttemptFailure {
	if sessionRef == nil && len(progress) == 0 {
		return failure
	}
	declared := providers.ExecuteFailure{Kind: providers.ExecuteFailureKindUnknown}
	if failure.Declared != nil {
		declared = failure.Declared.Clone()
	} else if failure.DecodeError != nil {
		declared.Kind = providers.ExecuteFailureKindInvalidRequest
		declared.Diagnostics = &providers.ExecuteDiagnostics{
			Metadata: map[string]string{"failure_stage": failureStageDecode},
		}
	}
	if sessionRef != nil {
		declared.SessionRef = sessionRef
	}
	if len(progress) > 0 {
		if declared.Diagnostics == nil {
			declared.Diagnostics = &providers.ExecuteDiagnostics{Progress: progress}
		} else {
			declared.Diagnostics.Progress = progress
		}
	}
	failure.Declared = &declared
	return failure
}

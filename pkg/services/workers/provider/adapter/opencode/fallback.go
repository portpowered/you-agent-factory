package opencode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
)

const (
	maxUnsupportedRejectionBytes = 4096
	maxSafeVersionLength         = 48
)

func (a *NegotiatedAdapter) PlanFallback(ctx context.Context, input adapter.FallbackContext) (adapter.FallbackPlan, bool, error) {
	if err := ctx.Err(); err != nil || a.decision.Mode != ModeStructured || a.resolver == nil {
		return adapter.FallbackPlan{}, false, err
	}
	if a.requireStructured {
		return adapter.FallbackPlan{}, false, nil
	}
	if !safeUnsupportedStructuredRejection(input) {
		return adapter.FallbackPlan{}, false, nil
	}
	downgraded, err := a.resolver.Downgrade(a.decision)
	if err != nil {
		return adapter.FallbackPlan{}, false, err
	}
	fallback, err := NewNegotiatedAdapter(downgraded, a.resolver)
	if err != nil {
		return adapter.FallbackPlan{}, false, err
	}
	diagnostic := adapter.Diagnostic{
		Code: "structured_mode_degraded",
		Message: fmt.Sprintf(
			"OpenCode selected final_only after unsupported_format rejection (version %s).",
			safeVersionContext(downgraded.Version),
		),
	}
	return adapter.FallbackPlan{Adapter: fallback, Diagnostic: diagnostic}, true, nil
}

func safeUnsupportedStructuredRejection(input adapter.FallbackContext) bool {
	if !safePreWorkFailure(input) {
		return false
	}
	message := strings.ToLower(strings.Join(strings.Fields(string(input.CommandResult.Stderr)), " "))
	return positiveUnsupportedFormatSignal(message)
}

func safePreWorkFailure(input adapter.FallbackContext) bool {
	if input.CommandError != nil || input.DecodeError != nil || input.FlushError != nil {
		return false
	}
	if input.FlushReason != adapter.FlushReasonTerminated || input.CommandResult.ExitCode == 0 {
		return false
	}
	if len(input.Drafts) != 0 || len(bytes.TrimSpace(input.CommandResult.Stdout)) != 0 {
		return false
	}
	stderrBytes := len(input.CommandResult.Stderr)
	return stderrBytes > 0 && stderrBytes <= maxUnsupportedRejectionBytes && errors.Is(input.ParseError, errMissingFinalSnapshot)
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
		(strings.Contains(message, "invalid value") || strings.Contains(message, "not supported") || strings.Contains(message, "unsupported"))
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

var _ adapter.FallbackPlanner = (*NegotiatedAdapter)(nil)

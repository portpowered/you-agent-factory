package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

type s14ProviderCase struct {
	provider       interfaces.ModelProvider
	unsafeMarker   string
	unsafeReq      interfaces.ProviderInferenceRequest
	safeReq        interfaces.ProviderInferenceRequest
	unsafeArgCheck func(args []string) bool
	safeArgCheck   func(args []string) bool
}

func s14SkipPermissionsProviderCases() []s14ProviderCase {
	return []s14ProviderCase{
		{
			provider:     interfaces.ModelProviderClaude,
			unsafeMarker: "--dangerously-skip-permissions",
			unsafeReq: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderClaude),
				UserMessage:   "run the tests",
			},
			safeReq: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderClaude),
				UserMessage:   "run the tests",
			},
			unsafeArgCheck: func(args []string) bool {
				return strings.Contains(strings.Join(args, " "), "--dangerously-skip-permissions")
			},
			safeArgCheck: func(args []string) bool {
				return !strings.Contains(strings.Join(args, " "), "--dangerously-skip-permissions")
			},
		},
		{
			provider:     interfaces.ModelProviderCodex,
			unsafeMarker: "--dangerously-bypass-approvals-and-sandbox",
			unsafeReq: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderCodex),
				UserMessage:   "run the tests",
			},
			safeReq: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderCodex),
				UserMessage:   "run the tests",
			},
			unsafeArgCheck: func(args []string) bool {
				return strings.Contains(strings.Join(args, " "), "--dangerously-bypass-approvals-and-sandbox")
			},
		},
		{
			provider:     interfaces.ModelProviderGemini,
			unsafeMarker: "--approval-mode",
			unsafeReq: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderGemini),
				UserMessage:   "run the tests",
			},
			safeReq: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderGemini),
				UserMessage:   "run the tests",
			},
			unsafeArgCheck: func(args []string) bool {
				joined := strings.Join(args, " ")
				return strings.Contains(joined, "--approval-mode") && strings.Contains(joined, "yolo") &&
					strings.Contains(joined, "--sandbox") && strings.Contains(joined, "false")
			},
		},
		{
			provider:     interfaces.ModelProviderKiro,
			unsafeMarker: "--trust-all-tools",
			unsafeReq: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderKiro),
				UserMessage:   "run the tests",
			},
			safeReq: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderKiro),
				UserMessage:   "run the tests",
			},
			unsafeArgCheck: func(args []string) bool {
				return strings.Contains(strings.Join(args, " "), "--trust-all-tools")
			},
		},
		{
			provider:     interfaces.ModelProviderCursor,
			unsafeMarker: "-f",
			unsafeReq: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderCursor),
				UserMessage:   "run the tests",
			},
			safeReq: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderCursor),
				UserMessage:   "run the tests",
			},
			unsafeArgCheck: func(args []string) bool {
				return len(args) > 0 && args[0] == "-f"
			},
		},
		{
			provider:     interfaces.ModelProviderOpenCode,
			unsafeMarker: "--dangerously-skip-permissions",
			unsafeReq: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderOpenCode),
				UserMessage:   "run the tests",
			},
			safeReq: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderOpenCode),
				UserMessage:   "run the tests",
			},
			unsafeArgCheck: func(args []string) bool {
				return strings.Contains(strings.Join(args, " "), "--dangerously-skip-permissions")
			},
		},
	}
}

func s14ResolveSafeArgCheck(tc *s14ProviderCase) {
	if tc.safeArgCheck != nil {
		return
	}
	marker := tc.unsafeMarker
	tc.safeArgCheck = func(args []string) bool {
		return !strings.Contains(strings.Join(args, " "), marker)
	}
	if tc.provider == interfaces.ModelProviderCursor {
		tc.safeArgCheck = func(args []string) bool {
			return len(args) == 0 || args[0] != "-f"
		}
	}
}

func assertS14ProviderUnsafeArgs(t *testing.T, tc s14ProviderCase) {
	t.Helper()
	behavior := providerBehaviorFor(string(tc.provider), logging.NoopLogger{})
	args, err := behavior.BuildArgs(context.Background(), tc.unsafeReq, true, nil)
	if err != nil {
		t.Fatalf("BuildArgs(skip=true): %v", err)
	}
	if !tc.unsafeArgCheck(args) {
		t.Fatalf("provider args = %#v, want unsafe marker %q", args, tc.unsafeMarker)
	}
}

func assertS14ProviderSafeArgs(t *testing.T, tc s14ProviderCase) {
	t.Helper()
	behavior := providerBehaviorFor(string(tc.provider), logging.NoopLogger{})
	args, err := behavior.BuildArgs(context.Background(), tc.safeReq, false, nil)
	if err != nil {
		t.Fatalf("BuildArgs(skip=false): %v", err)
	}
	if !tc.safeArgCheck(args) {
		t.Fatalf("provider args = %#v, want to omit unsafe marker %q", args, tc.unsafeMarker)
	}
}

func TestS14SupportedProviderUnsafeOptionPropagationEvidence(t *testing.T) {
	t.Parallel()

	for _, tc := range s14SkipPermissionsProviderCases() {
		tc := tc
		s14ResolveSafeArgCheck(&tc)
		t.Run(string(tc.provider)+"/EffectiveTrueIncludesUnsafeOption", func(t *testing.T) {
			t.Parallel()
			assertS14ProviderUnsafeArgs(t, tc)
		})
		t.Run(string(tc.provider)+"/EffectiveFalseOmitsUnsafeOption", func(t *testing.T) {
			t.Parallel()
			assertS14ProviderSafeArgs(t, tc)
		})
	}
}

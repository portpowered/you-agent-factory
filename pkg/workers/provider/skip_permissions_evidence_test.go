package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

func TestS14SupportedProviderUnsafeOptionPropagationEvidence(t *testing.T) {
	t.Parallel()

	type providerCase struct {
		provider       interfaces.ModelProvider
		unsafeMarker   string
		unsafeReq      interfaces.ProviderInferenceRequest
		safeReq        interfaces.ProviderInferenceRequest
		unsafeArgCheck func(args []string) bool
		safeArgCheck   func(args []string) bool
	}

	cases := []providerCase{
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

	for _, tc := range cases {
		tc := tc
		if tc.safeArgCheck == nil {
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
		t.Run(string(tc.provider)+"/EffectiveTrueIncludesUnsafeOption", func(t *testing.T) {
			t.Parallel()
			behavior := providerBehaviorFor(string(tc.provider), logging.NoopLogger{})
			args, err := behavior.BuildArgs(context.Background(), tc.unsafeReq, true, nil)
			if err != nil {
				t.Fatalf("BuildArgs(skip=true): %v", err)
			}
			if !tc.unsafeArgCheck(args) {
				t.Fatalf("provider args = %#v, want unsafe marker %q", args, tc.unsafeMarker)
			}
		})
		t.Run(string(tc.provider)+"/EffectiveFalseOmitsUnsafeOption", func(t *testing.T) {
			t.Parallel()
			behavior := providerBehaviorFor(string(tc.provider), logging.NoopLogger{})
			args, err := behavior.BuildArgs(context.Background(), tc.safeReq, false, nil)
			if err != nil {
				t.Fatalf("BuildArgs(skip=false): %v", err)
			}
			if !tc.safeArgCheck(args) {
				t.Fatalf("provider args = %#v, want to omit unsafe marker %q", args, tc.unsafeMarker)
			}
		})
	}
}

package workflowpolicy_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
)

func TestValidateChildRequest_AllowlistAndCapabilityDenials(t *testing.T) {
	base := workflowpolicy.DefaultEffectivePolicy()
	base.AllowedModels = []string{"gpt-allowed"}
	base.AllowedReasoningEfforts = []string{"low"}
	base.AllowedCommands = []string{"review"}
	base.Concurrency = 2

	cases := []struct {
		name    string
		policy  workflowpolicy.EffectivePolicy
		request workflowpolicy.ChildRequest
		want    string
	}{
		{
			name:   "model",
			policy: base,
			request: workflowpolicy.ChildRequest{
				Label: "child-a",
				Model: "gpt-denied",
			},
			want: "policy denied: model \"gpt-denied\" is not listed in allowedModels",
		},
		{
			name:   "reasoningEffort",
			policy: base,
			request: workflowpolicy.ChildRequest{
				Label:           "child-a",
				ReasoningEffort: "high",
			},
			want: "policy denied: reasoningEffort \"high\" is not listed in allowedReasoningEfforts",
		},
		{
			name:   "command",
			policy: base,
			request: workflowpolicy.ChildRequest{
				Label:   "child-a",
				Command: "deploy",
			},
			want: "policy denied: command \"deploy\" is not listed in allowedCommands",
		},
		{
			name:   "sandbox",
			policy: base,
			request: workflowpolicy.ChildRequest{
				Label:   "child-a",
				Sandbox: "workspace-write",
			},
			want: "policy denied: sandbox \"workspace-write\" is not allowed when policy.mode is READ_ONLY",
		},
		{
			name:   "writableRoots",
			policy: base,
			request: workflowpolicy.ChildRequest{
				Label:         "child-a",
				WritableRoots: []string{"/tmp/out"},
			},
			want: "policy denied: writableRoots are not allowed by effective policy",
		},
		{
			name:   "network",
			policy: base,
			request: workflowpolicy.ChildRequest{
				Label:        "child-a",
				AllowNetwork: true,
			},
			want: "policy denied: network access is not allowed by effective policy",
		},
		{
			name:   "concurrency",
			policy: base,
			request: workflowpolicy.ChildRequest{
				Label:       "child-a",
				Concurrency: 4,
			},
			want: "policy denied: requested concurrency 4 exceeds policy concurrency 2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := workflowpolicy.ValidateChildRequest(tc.policy, tc.request)
			if err == nil {
				t.Fatal("expected policy denial")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
			if !strings.Contains(err.Error(), "label=") {
				t.Fatalf("error = %q, want safe request context", err.Error())
			}
		})
	}
}

func TestValidateChildRequest_AllowsPermittedRequest(t *testing.T) {
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.AllowedModels = []string{"gpt-allowed"}
	policy.AllowedReasoningEfforts = []string{"low"}
	policy.AllowedCommands = []string{"review"}
	policy.Concurrency = 4

	err := workflowpolicy.ValidateChildRequest(policy, workflowpolicy.ChildRequest{
		Label:           "child-a",
		Model:           "gpt-allowed",
		ReasoningEffort: "low",
		Command:         "review",
		Sandbox:         "read-only",
		Concurrency:     2,
	})
	if err != nil {
		t.Fatalf("ValidateChildRequest() error = %v, want nil", err)
	}
}

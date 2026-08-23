package factory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestValidateChildRequest_AllowlistAndBudgetDenials(t *testing.T) {
	t.Parallel()
	base := factory.DefaultJavaScriptPolicy()
	base.AllowedModels = []string{"gpt-allowed"}
	base.AllowedReasoningEfforts = []string{"low"}
	base.AllowedCommands = []string{"review"}
	base.Concurrency = 2

	cases := []struct {
		name    string
		policy  factory.JavaScriptPolicy
		request factory.JavaScriptPolicyChildRequest
		want    string
	}{
		{
			name:   "model",
			policy: base,
			request: factory.JavaScriptPolicyChildRequest{
				Label: "child-a",
				Model: "gpt-denied",
			},
			want: "policy denied: model \"gpt-denied\" is not listed in allowedModels",
		},
		{
			name:   "reasoningEffort",
			policy: base,
			request: factory.JavaScriptPolicyChildRequest{
				Label:           "child-a",
				ReasoningEffort: "high",
			},
			want: "policy denied: reasoningEffort \"high\" is not listed in allowedReasoningEfforts",
		},
		{
			name:   "command",
			policy: base,
			request: factory.JavaScriptPolicyChildRequest{
				Label:   "child-a",
				Command: "deploy",
			},
			want: "policy denied: command \"deploy\" is not listed in allowedCommands",
		},
		{
			name:   "concurrency",
			policy: base,
			request: factory.JavaScriptPolicyChildRequest{
				Label:       "child-a",
				Concurrency: 4,
			},
			want: "policy denied: requested concurrency 4 exceeds policy concurrency 2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := factory.ValidateJavaScriptPolicyChildRequest(tc.policy, tc.request)
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

func TestValidateChildRequest_AllowedPermissionsNamesFactoryChildAndRequestedValue(t *testing.T) {
	t.Parallel()
	policy := factory.DefaultJavaScriptPolicy()
	policy.AllowedPermissions = []string{factory.JavaScriptPolicyPermissionDefault}
	request := factory.JavaScriptPolicyChildRequest{
		FactoryName:     "named-factory",
		Label:           "skip-child",
		SkipPermissions: true,
	}

	err := factory.ValidateJavaScriptPolicyChildRequest(policy, request)
	const want = `policy denied: Factory "named-factory" child "skip-child" requested permission "SKIP_PERMISSIONS" not listed in allowedPermissions`
	if err == nil || err.Error() != want {
		t.Fatalf("ValidateChildRequest() error = %v, want exact diagnostic %q", err, want)
	}

	request.SkipPermissions = false
	if err := factory.ValidateJavaScriptPolicyChildRequest(policy, request); err != nil {
		t.Fatalf("ValidateChildRequest() default permission error = %v, want nil", err)
	}

	policy.AllowedPermissions = nil
	request.SkipPermissions = true
	if err := factory.ValidateJavaScriptPolicyChildRequest(policy, request); err != nil {
		t.Fatalf("ValidateChildRequest() omitted allowlist error = %v, want nil", err)
	}

	policy.AllowedPermissions = []string{factory.JavaScriptPolicyPermissionSkipPermissions}
	if err := factory.ValidateJavaScriptPolicyChildRequest(policy, request); err != nil {
		t.Fatalf("ValidateChildRequest() skip permission error = %v, want nil", err)
	}
}

func TestValidateChildRequest_AllowsPermittedRequest(t *testing.T) {
	t.Parallel()
	policy := factory.DefaultJavaScriptPolicy()
	policy.AllowedModels = []string{"gpt-allowed"}
	policy.AllowedReasoningEfforts = []string{"low"}
	policy.AllowedCommands = []string{"review"}
	policy.Concurrency = 4

	err := factory.ValidateJavaScriptPolicyChildRequest(policy, factory.JavaScriptPolicyChildRequest{
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

func TestValidateChildRequest_SkipPermissionsDoesNotBypassRuntimeAllowlists(t *testing.T) {
	t.Parallel()
	policy := factory.DefaultJavaScriptPolicy()
	policy.AllowedModels = []string{"gpt-allowed"}
	policy.AllowedReasoningEfforts = []string{"low"}
	policy.AllowedCommands = []string{"review"}
	policy.Concurrency = 2

	tests := []struct {
		name    string
		request factory.JavaScriptPolicyChildRequest
		wantErr string
	}{
		{
			name: "workspace write sandbox",
			request: factory.JavaScriptPolicyChildRequest{
				Label:           "autonomous-child",
				SkipPermissions: true,
				Sandbox:         "workspace-write",
			},
		},
		{
			name: "model allowlist remains enforced",
			request: factory.JavaScriptPolicyChildRequest{
				Label:           "autonomous-child",
				Model:           "gpt-denied",
				SkipPermissions: true,
			},
			wantErr: `policy denied: model "gpt-denied" is not listed in allowedModels`,
		},
		{
			name: "reasoning allowlist remains enforced",
			request: factory.JavaScriptPolicyChildRequest{
				Label:           "autonomous-child",
				ReasoningEffort: "high",
				SkipPermissions: true,
			},
			wantErr: `policy denied: reasoningEffort "high" is not listed in allowedReasoningEfforts`,
		},
		{
			name: "command allowlist remains enforced",
			request: factory.JavaScriptPolicyChildRequest{
				Label:           "autonomous-child",
				Command:         "deploy",
				SkipPermissions: true,
			},
			wantErr: `policy denied: command "deploy" is not listed in allowedCommands`,
		},
		{
			name: "concurrency budget remains enforced",
			request: factory.JavaScriptPolicyChildRequest{
				Label:           "autonomous-child",
				Concurrency:     3,
				SkipPermissions: true,
			},
			wantErr: "policy denied: requested concurrency 3 exceeds policy concurrency 2",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := factory.ValidateJavaScriptPolicyChildRequest(policy, test.request)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateChildRequest() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateChildRequest() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

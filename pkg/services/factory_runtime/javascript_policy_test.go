package factory_test

import (
	"encoding/json"
	"reflect"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestDefaultEffectivePolicy_MatchesReadOnlyMVPDefaults(t *testing.T) {
	t.Parallel()
	policy := factory.DefaultJavaScriptPolicy()
	if policy.Mode != factory.JavaScriptPolicyModeReadOnly {
		t.Fatalf("mode = %q, want %q", policy.Mode, factory.JavaScriptPolicyModeReadOnly)
	}
	if policy.MaxAgents != factory.DefaultJavaScriptPolicyMaxAgents {
		t.Fatalf("maxAgents = %d, want %d", policy.MaxAgents, factory.DefaultJavaScriptPolicyMaxAgents)
	}
	if policy.Concurrency != 4 {
		t.Fatalf("concurrency = %d, want 4", policy.Concurrency)
	}
	if policy.MaxDepth != 1 || policy.MaxRetries != 0 {
		t.Fatalf("depth/retries = %d/%d, want 1/0", policy.MaxDepth, policy.MaxRetries)
	}
	if policy.AllowNetwork || policy.AllowConnectors || policy.AllowDangerFullAccess {
		t.Fatalf("host capabilities should be denied by default: %#v", policy)
	}
	if len(policy.WritableRoots) != 0 {
		t.Fatalf("writableRoots = %#v, want empty", policy.WritableRoots)
	}
	if policy.OutputAuditMode != factory.JavaScriptPolicyOutputAuditModeAuto {
		t.Fatalf("outputAuditMode = %q, want AUTO", policy.OutputAuditMode)
	}
}

func TestHash_StableAcrossMapOrdering(t *testing.T) {
	t.Parallel()
	first := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
		FactoryDefault: json.RawMessage(`{"mode":"READ_ONLY","maxAgents":16,"concurrency":4}`),
	})
	second := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
		FactoryDefault: json.RawMessage(`{"concurrency":4,"maxAgents":16,"mode":"READ_ONLY"}`),
	})
	if first.Hash == "" || second.Hash == "" {
		t.Fatalf("policy hashes = %q / %q, want non-empty digests", first.Hash, second.Hash)
	}
	if first.Hash != second.Hash {
		t.Fatalf("policy hashes differ for equivalent policy maps: %q vs %q", first.Hash, second.Hash)
	}
}

func TestValidate_RejectsInvalidConcurrencyAndExcessiveMaxAgents(t *testing.T) {
	t.Parallel()
	invalidConcurrency := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
		Requested: map[string]any{"concurrency": 0},
	})
	if len(invalidConcurrency.Issues) == 0 {
		t.Fatal("expected invalid concurrency issue")
	}
	if invalidConcurrency.Issues[0].Code != factory.JavaScriptPolicyCodeInvalidConcurrency {
		t.Fatalf("issue code = %q, want %q", invalidConcurrency.Issues[0].Code, factory.JavaScriptPolicyCodeInvalidConcurrency)
	}

	aboveMaxAgents := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
		Requested: map[string]any{"maxAgents": 4, "concurrency": 8},
	})
	found := false
	for _, issue := range aboveMaxAgents.Issues {
		if issue.Code == factory.JavaScriptPolicyCodeConcurrencyAboveMaxAgents {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want concurrencyAboveMaxAgents", aboveMaxAgents.Issues)
	}

	excessive := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
		Requested:     map[string]any{"maxAgents": 2000},
		DeploymentCap: 1000,
	})
	found = false
	for _, issue := range excessive.Issues {
		if issue.Code == factory.JavaScriptPolicyCodeExcessiveMaxAgents {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want excessiveMaxAgents", excessive.Issues)
	}
}

func TestValidate_RejectsUnsupportedPolicyModeOverrides(t *testing.T) {
	t.Parallel()
	unsafeMode := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
		Requested: map[string]any{
			"mode":                  "WRITE",
			"allowNetwork":          true,
			"allowConnectors":       true,
			"allowDangerFullAccess": true,
		},
	})
	foundMode := false
	foundDenied := false
	for _, issue := range unsafeMode.Issues {
		switch issue.Code {
		case factory.JavaScriptPolicyCodeUnsupportedPolicyMode:
			foundMode = true
		case factory.JavaScriptPolicyCodeDeniedCapability:
			foundDenied = true
		}
	}
	if !foundMode {
		t.Fatalf("issues = %#v, want unsupportedMode for non-READ_ONLY policy.mode", unsafeMode.Issues)
	}
	if !foundDenied {
		t.Fatalf("issues = %#v, want denied capability diagnostics for unsafe flags", unsafeMode.Issues)
	}
	if unsafeMode.Policy.Mode == factory.JavaScriptPolicyModeReadOnly && len(unsafeMode.Issues) == 0 {
		t.Fatal("unsafe policy overrides should not produce a valid read-only effective policy")
	}
}

func TestValidate_RejectsWritableRootsAndUnknownRunnerUnderReadOnly(t *testing.T) {
	t.Parallel()
	writableRoots := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
		Requested: map[string]any{"writableRoots": []any{"/tmp/out"}},
	})
	found := false
	for _, issue := range writableRoots.Issues {
		if issue.Code == factory.JavaScriptPolicyCodeWritableRootsReadOnly {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want writableRootsReadOnly", writableRoots.Issues)
	}

	unknownRunner := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
		Requested: map[string]any{"allowedRunners": []any{"not-a-runner"}},
	})
	found = false
	for _, issue := range unknownRunner.Issues {
		if issue.Code == factory.JavaScriptPolicyCodeUnsupportedRunner {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want unsupportedRunner", unknownRunner.Issues)
	}
}

func TestValidateCapability_ReadOnlyDeniesHostCapabilitiesBeforeRuntime(t *testing.T) {
	t.Parallel()
	policy := factory.DefaultJavaScriptPolicy()
	capabilities := []factory.JavaScriptPolicyCapability{
		factory.JavaScriptPolicyCapabilityWorkspaceWrite,
		factory.JavaScriptPolicyCapabilityFilesystemWrite,
		factory.JavaScriptPolicyCapabilityShellProcess,
		factory.JavaScriptPolicyCapabilityNetwork,
		factory.JavaScriptPolicyCapabilityConnectors,
		factory.JavaScriptPolicyCapabilityDangerFullAccess,
	}
	for _, capability := range capabilities {
		if diagnostic := factory.ValidateJavaScriptPolicyCapability(policy, capability); diagnostic == nil {
			t.Fatalf("capability %q should be denied for read-only policy", capability)
		}
	}
}

func TestDeniedJavaScriptPolicyCapabilities_CharacterizesCurrentDiagnostics(t *testing.T) {
	t.Parallel()

	wantDefault := []factory.JavaScriptPolicyDiagnostic{
		{Code: factory.JavaScriptPolicyCodeDeniedCapability, Message: "workspace-write workers are denied when policy.mode is READ_ONLY (workspace-write)"},
		{Code: factory.JavaScriptPolicyCodeDeniedCapability, Message: "direct workflow filesystem writes are denied when policy.mode is READ_ONLY (filesystem-write)"},
		{Code: factory.JavaScriptPolicyCodeDeniedCapability, Message: "direct shell/process access is denied when policy.mode is READ_ONLY (shell-process)"},
		{Code: factory.JavaScriptPolicyCodeDeniedCapability, Message: "direct network access is denied when policy.mode is READ_ONLY (network)"},
		{Code: factory.JavaScriptPolicyCodeDeniedCapability, Message: "connectors are denied when policy.mode is READ_ONLY (connectors)"},
		{Code: factory.JavaScriptPolicyCodeDeniedCapability, Message: "danger-full-access is denied when policy.mode is READ_ONLY (danger-full-access)"},
	}

	tests := []struct {
		name   string
		policy factory.JavaScriptPolicy
		want   []factory.JavaScriptPolicyDiagnostic
	}{
		{
			name:   "read-only defaults",
			policy: factory.DefaultJavaScriptPolicy(),
			want:   wantDefault,
		},
		{
			name: "read-only allow flags omit only their capabilities",
			policy: func() factory.JavaScriptPolicy {
				policy := factory.DefaultJavaScriptPolicy()
				policy.AllowNetwork = true
				policy.AllowConnectors = true
				policy.AllowDangerFullAccess = true
				return policy
			}(),
			want: wantDefault[:3],
		},
		{
			name: "non-read-only mode",
			policy: func() factory.JavaScriptPolicy {
				policy := factory.DefaultJavaScriptPolicy()
				policy.Mode = "WRITE"
				return policy
			}(),
			want: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := factory.DeniedJavaScriptPolicyCapabilities(test.policy)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("DeniedJavaScriptPolicyCapabilities() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestBuildPreview_IncludesEffectivePolicyHashBudgetsAndDeniedCapabilities(t *testing.T) {
	t.Parallel()
	preview := factory.BuildJavaScriptPolicyPreview(factory.JavaScriptPolicyPreviewInput{
		Request: factory.JavaScriptPolicyRequest{
			Requested: map[string]any{"maxAgents": 8},
		},
		RequestedRunner:  workerexecution.RunnerIDCodex,
		RequestedModel:   "gpt-5-codex",
		RequestedProfile: "reviewer",
	})
	if preview.PolicyHash == "" {
		t.Fatal("expected policy hash in preview")
	}
	if preview.MaxChildCount != 8 || preview.MaxConcurrency != 4 {
		t.Fatalf("child/concurrency = %d/%d, want 8/4", preview.MaxChildCount, preview.MaxConcurrency)
	}
	if preview.EffectivePolicy.Mode != factory.JavaScriptPolicyModeReadOnly {
		t.Fatalf("effective policy mode = %q, want READ_ONLY", preview.EffectivePolicy.Mode)
	}
	if len(preview.DeniedCapabilities) == 0 {
		t.Fatal("expected denied capability diagnostics for read-only preview")
	}
	if preview.RunnerDecision == nil || !preview.RunnerDecision.Allowed {
		t.Fatalf("runner decision = %#v, want allowed codex runner", preview.RunnerDecision)
	}
	if preview.ModelDecision == nil || !preview.ModelDecision.Allowed {
		t.Fatalf("model decision = %#v, want allowed model", preview.ModelDecision)
	}
	if preview.ProfileDecision == nil || !preview.ProfileDecision.Allowed {
		t.Fatalf("profile decision = %#v, want allowed reviewer profile", preview.ProfileDecision)
	}
}

func TestOrchestratorTargets_RejectsInvalidDefaultPolicy(t *testing.T) {
	t.Parallel()
	cfg := &interfaces.FactoryConfig{
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				SourceRef:     "factory/workflows/review.js",
				DefaultPolicy: json.RawMessage(`{"maxAgents":2000}`),
			},
		},
	}
	targets := factoryruntimewire.NewOrchestratorDefinitionValidator(testJavaScriptWorkflows()).
		ValidateJavaScriptFactoryDefinition(
			t.Context(),
			cfg.Orchestrator.JavaScript,
			nil,
		)
	found := false
	for _, target := range targets {
		if target.Code == factory.JavaScriptPolicyCodeExcessiveMaxAgents {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want excessiveMaxAgents from orchestrator policy validation", targets)
	}
}

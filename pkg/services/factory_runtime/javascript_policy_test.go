package factory_test

import (
	"encoding/json"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestDefaultEffectivePolicy_ContainsOnlySurvivingPolicyFields(t *testing.T) {
	t.Parallel()
	policy := factory.DefaultJavaScriptPolicy()
	if policy.MaxAgents != factory.DefaultJavaScriptPolicyMaxAgents {
		t.Fatalf("maxAgents = %d, want %d", policy.MaxAgents, factory.DefaultJavaScriptPolicyMaxAgents)
	}
	if policy.Concurrency != 4 {
		t.Fatalf("concurrency = %d, want 4", policy.Concurrency)
	}
	if policy.MaxDepth != 1 || policy.MaxRetries != 0 {
		t.Fatalf("depth/retries = %d/%d, want 1/0", policy.MaxDepth, policy.MaxRetries)
	}
	if len(policy.AllowedPermissions) != 0 {
		t.Fatalf("allowedPermissions = %#v, want omitted by default", policy.AllowedPermissions)
	}
	if policy.OutputAuditMode != factory.JavaScriptPolicyOutputAuditModeAuto {
		t.Fatalf("outputAuditMode = %q, want AUTO", policy.OutputAuditMode)
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode policy: %v", err)
	}
	for _, retired := range []string{"mode", "allowNetwork", "allowConnectors", "allowDangerFullAccess", "writableRoots"} {
		if _, ok := fields[retired]; ok {
			t.Fatalf("default policy contains retired field %q: %s", retired, encoded)
		}
	}
}

func TestHash_StableAcrossMapOrdering(t *testing.T) {
	t.Parallel()
	first := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
		FactoryDefault: json.RawMessage(`{"maxAgents":16,"concurrency":4}`),
	})
	second := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
		FactoryDefault: json.RawMessage(`{"concurrency":4,"maxAgents":16}`),
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

func TestValidate_RejectsRetiredPolicyFields(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"mode", "allowNetwork", "allowConnectors", "allowDangerFullAccess", "writableRoots"} {
		resolution := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
			Requested: map[string]any{field: true},
		})
		found := false
		for _, issue := range resolution.Issues {
			if issue.Code == factory.JavaScriptPolicyCodeUnsupportedPolicyField && issue.Path == "policy."+field &&
				strings.Contains(issue.Message, "allowedPermissions") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("field %q issues = %#v, want retired-field diagnostic", field, resolution.Issues)
		}
	}

	unknownRunner := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
		Requested: map[string]any{"allowedRunners": []any{"not-a-runner"}},
	})
	found := false
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

func TestResolve_AllowedPermissionsAcceptsCanonicalValues(t *testing.T) {
	t.Parallel()
	for _, allowed := range []string{
		factory.JavaScriptPolicyPermissionDefault,
		factory.JavaScriptPolicyPermissionSkipPermissions,
	} {
		resolution := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
			Requested: map[string]any{"allowedPermissions": []any{allowed}},
		})
		if len(resolution.Issues) != 0 || len(resolution.Policy.AllowedPermissions) != 1 || resolution.Policy.AllowedPermissions[0] != allowed {
			t.Fatalf("allowedPermissions %q resolution = %#v, want valid canonical value", allowed, resolution)
		}
	}
	invalid := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{
		Requested: map[string]any{"allowedPermissions": []any{"WRITE"}},
	})
	for _, issue := range invalid.Issues {
		if issue.Code == factory.JavaScriptPolicyCodeUnsupportedPermission && issue.Path == "policy.allowedPermissions[0]" {
			return
		}
	}
	t.Fatalf("invalid allowedPermissions issues = %#v, want indexed unsupported permission", invalid.Issues)
}

func TestBuildPreview_IncludesEffectivePolicyHashBudgetsAndAllowlistDecisions(t *testing.T) {
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

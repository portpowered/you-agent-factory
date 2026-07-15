package workflowpolicy_test

import (
	"encoding/json"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestDefaultEffectivePolicy_MatchesReadOnlyMVPDefaults(t *testing.T) {
	policy := workflowpolicy.DefaultEffectivePolicy()
	if policy.Mode != workflowpolicy.ModeReadOnly {
		t.Fatalf("mode = %q, want %q", policy.Mode, workflowpolicy.ModeReadOnly)
	}
	if policy.MaxAgents != workflowpolicy.DefaultMaxAgents {
		t.Fatalf("maxAgents = %d, want %d", policy.MaxAgents, workflowpolicy.DefaultMaxAgents)
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
	if policy.OutputAuditMode != workflowpolicy.OutputAuditModeAuto {
		t.Fatalf("outputAuditMode = %q, want AUTO", policy.OutputAuditMode)
	}
}

func TestHash_StableAcrossMapOrdering(t *testing.T) {
	first := workflowpolicy.Resolve(workflowpolicy.Request{
		FactoryDefault: json.RawMessage(`{"mode":"READ_ONLY","maxAgents":16,"concurrency":4}`),
	})
	second := workflowpolicy.Resolve(workflowpolicy.Request{
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
	invalidConcurrency := workflowpolicy.Resolve(workflowpolicy.Request{
		Requested: map[string]any{"concurrency": 0},
	})
	if len(invalidConcurrency.Issues) == 0 {
		t.Fatal("expected invalid concurrency issue")
	}
	if invalidConcurrency.Issues[0].Code != workflowpolicy.CodeInvalidConcurrency {
		t.Fatalf("issue code = %q, want %q", invalidConcurrency.Issues[0].Code, workflowpolicy.CodeInvalidConcurrency)
	}

	aboveMaxAgents := workflowpolicy.Resolve(workflowpolicy.Request{
		Requested: map[string]any{"maxAgents": 4, "concurrency": 8},
	})
	found := false
	for _, issue := range aboveMaxAgents.Issues {
		if issue.Code == workflowpolicy.CodeConcurrencyAboveMaxAgents {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want concurrencyAboveMaxAgents", aboveMaxAgents.Issues)
	}

	excessive := workflowpolicy.Resolve(workflowpolicy.Request{
		Requested:     map[string]any{"maxAgents": 2000},
		DeploymentCap: 1000,
	})
	found = false
	for _, issue := range excessive.Issues {
		if issue.Code == workflowpolicy.CodeExcessiveMaxAgents {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want excessiveMaxAgents", excessive.Issues)
	}
}

func TestValidate_RejectsUnsupportedPolicyModeOverrides(t *testing.T) {
	unsafeMode := workflowpolicy.Resolve(workflowpolicy.Request{
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
		case workflowpolicy.CodeUnsupportedPolicyMode:
			foundMode = true
		case workflowpolicy.CodeDeniedCapability:
			foundDenied = true
		}
	}
	if !foundMode {
		t.Fatalf("issues = %#v, want unsupportedMode for non-READ_ONLY policy.mode", unsafeMode.Issues)
	}
	if !foundDenied {
		t.Fatalf("issues = %#v, want denied capability diagnostics for unsafe flags", unsafeMode.Issues)
	}
	if unsafeMode.Policy.Mode == workflowpolicy.ModeReadOnly && len(unsafeMode.Issues) == 0 {
		t.Fatal("unsafe policy overrides should not produce a valid read-only effective policy")
	}
}

func TestValidate_RejectsWritableRootsAndUnknownRunnerUnderReadOnly(t *testing.T) {
	writableRoots := workflowpolicy.Resolve(workflowpolicy.Request{
		Requested: map[string]any{"writableRoots": []any{"/tmp/out"}},
	})
	found := false
	for _, issue := range writableRoots.Issues {
		if issue.Code == workflowpolicy.CodeWritableRootsReadOnly {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want writableRootsReadOnly", writableRoots.Issues)
	}

	unknownRunner := workflowpolicy.Resolve(workflowpolicy.Request{
		Requested: map[string]any{"allowedRunners": []any{"not-a-runner"}},
	})
	found = false
	for _, issue := range unknownRunner.Issues {
		if issue.Code == workflowpolicy.CodeUnsupportedRunner {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want unsupportedRunner", unknownRunner.Issues)
	}
}

func TestValidateCapability_ReadOnlyDeniesHostCapabilitiesBeforeRuntime(t *testing.T) {
	policy := workflowpolicy.DefaultEffectivePolicy()
	capabilities := []workflowpolicy.Capability{
		workflowpolicy.CapabilityWorkspaceWrite,
		workflowpolicy.CapabilityFilesystemWrite,
		workflowpolicy.CapabilityShellProcess,
		workflowpolicy.CapabilityNetwork,
		workflowpolicy.CapabilityConnectors,
		workflowpolicy.CapabilityDangerFullAccess,
	}
	for _, capability := range capabilities {
		if diagnostic := workflowpolicy.ValidateCapability(policy, capability); diagnostic == nil {
			t.Fatalf("capability %q should be denied for read-only policy", capability)
		}
	}
}

func TestBuildPreview_IncludesEffectivePolicyHashBudgetsAndDeniedCapabilities(t *testing.T) {
	preview := workflowpolicy.BuildPreview(workflowpolicy.PreviewInput{
		Request: workflowpolicy.Request{
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
	if preview.EffectivePolicy.Mode != workflowpolicy.ModeReadOnly {
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
	cfg := &interfaces.FactoryConfig{
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				SourceRef:     "factory/workflows/review.js",
				DefaultPolicy: json.RawMessage(`{"maxAgents":2000}`),
			},
		},
	}
	targets := factoryvalidation.OrchestratorTargets(cfg)
	found := false
	for _, target := range targets {
		if target.Code == workflowpolicy.CodeExcessiveMaxAgents {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want excessiveMaxAgents from orchestrator policy validation", targets)
	}
}

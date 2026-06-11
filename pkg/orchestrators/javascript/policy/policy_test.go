package policy_test

import (
	"encoding/json"
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	jspolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
)

func TestDefaultEffectivePolicy_MatchesReadOnlyMVPDefaults(t *testing.T) {
	effective := jspolicy.DefaultEffectivePolicy()
	if effective.Mode != jspolicy.ModeReadOnly {
		t.Fatalf("mode = %q, want %q", effective.Mode, jspolicy.ModeReadOnly)
	}
	if effective.MaxAgents != jspolicy.DefaultMaxAgents {
		t.Fatalf("maxAgents = %d, want %d", effective.MaxAgents, jspolicy.DefaultMaxAgents)
	}
	if effective.Concurrency != 4 {
		t.Fatalf("concurrency = %d, want 4", effective.Concurrency)
	}
	if effective.MaxDepth != 1 || effective.MaxRetries != 0 {
		t.Fatalf("depth/retries = %d/%d, want 1/0", effective.MaxDepth, effective.MaxRetries)
	}
	if effective.AllowNetwork || effective.AllowConnectors || effective.AllowDangerFullAccess {
		t.Fatalf("host capabilities should be denied by default: %#v", effective)
	}
	if len(effective.WritableRoots) != 0 {
		t.Fatalf("writableRoots = %#v, want empty", effective.WritableRoots)
	}
	if effective.OutputAuditMode != jspolicy.OutputAuditModeAuto {
		t.Fatalf("outputAuditMode = %q, want AUTO", effective.OutputAuditMode)
	}
}

func TestHash_StableAcrossMapOrdering(t *testing.T) {
	first := jspolicy.Resolve(jspolicy.Request{
		FactoryDefault: json.RawMessage(`{"mode":"READ_ONLY","maxAgents":16,"concurrency":4}`),
	})
	second := jspolicy.Resolve(jspolicy.Request{
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
	invalidConcurrency := jspolicy.Resolve(jspolicy.Request{
		Requested: map[string]any{"concurrency": 0},
	})
	if len(invalidConcurrency.Issues) == 0 {
		t.Fatal("expected invalid concurrency issue")
	}
	if invalidConcurrency.Issues[0].Code != jspolicy.CodeInvalidConcurrency {
		t.Fatalf("issue code = %q, want %q", invalidConcurrency.Issues[0].Code, jspolicy.CodeInvalidConcurrency)
	}

	aboveMaxAgents := jspolicy.Resolve(jspolicy.Request{
		Requested: map[string]any{"maxAgents": 4, "concurrency": 8},
	})
	found := false
	for _, issue := range aboveMaxAgents.Issues {
		if issue.Code == jspolicy.CodeConcurrencyAboveMaxAgents {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want concurrencyAboveMaxAgents", aboveMaxAgents.Issues)
	}

	excessive := jspolicy.Resolve(jspolicy.Request{
		Requested:     map[string]any{"maxAgents": 2000},
		DeploymentCap: 1000,
	})
	found = false
	for _, issue := range excessive.Issues {
		if issue.Code == jspolicy.CodeExcessiveMaxAgents {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want excessiveMaxAgents", excessive.Issues)
	}
}

func TestValidate_RejectsUnsupportedPolicyModeOverrides(t *testing.T) {
	unsafeMode := jspolicy.Resolve(jspolicy.Request{
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
		case jspolicy.CodeUnsupportedPolicyMode:
			foundMode = true
		case jspolicy.CodeDeniedCapability:
			foundDenied = true
		}
	}
	if !foundMode {
		t.Fatalf("issues = %#v, want unsupportedMode for non-READ_ONLY policy.mode", unsafeMode.Issues)
	}
	if !foundDenied {
		t.Fatalf("issues = %#v, want denied capability diagnostics for unsafe flags", unsafeMode.Issues)
	}
	if unsafeMode.Policy.Mode == jspolicy.ModeReadOnly && len(unsafeMode.Issues) == 0 {
		t.Fatal("unsafe policy overrides should not produce a valid read-only effective policy")
	}
}

func TestValidate_RejectsWritableRootsAndUnknownRunnerUnderReadOnly(t *testing.T) {
	writableRoots := jspolicy.Resolve(jspolicy.Request{
		Requested: map[string]any{"writableRoots": []any{"/tmp/out"}},
	})
	found := false
	for _, issue := range writableRoots.Issues {
		if issue.Code == jspolicy.CodeWritableRootsReadOnly {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want writableRootsReadOnly", writableRoots.Issues)
	}

	unknownRunner := jspolicy.Resolve(jspolicy.Request{
		Requested: map[string]any{"allowedRunners": []any{"not-a-runner"}},
	})
	found = false
	for _, issue := range unknownRunner.Issues {
		if issue.Code == jspolicy.CodeUnsupportedRunner {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want unsupportedRunner", unknownRunner.Issues)
	}
}

func TestValidateCapability_ReadOnlyDeniesHostCapabilitiesBeforeRuntime(t *testing.T) {
	effective := jspolicy.DefaultEffectivePolicy()
	capabilities := []jspolicy.Capability{
		jspolicy.CapabilityWorkspaceWrite,
		jspolicy.CapabilityFilesystemWrite,
		jspolicy.CapabilityShellProcess,
		jspolicy.CapabilityNetwork,
		jspolicy.CapabilityConnectors,
		jspolicy.CapabilityDangerFullAccess,
	}
	for _, capability := range capabilities {
		if diagnostic := jspolicy.ValidateCapability(effective, capability); diagnostic == nil {
			t.Fatalf("capability %q should be denied for read-only policy", capability)
		}
	}
}

func TestBuildPreview_IncludesEffectivePolicyHashBudgetsAndDeniedCapabilities(t *testing.T) {
	preview := jspolicy.BuildPreview(jspolicy.PreviewInput{
		Request: jspolicy.Request{
			Requested: map[string]any{"maxAgents": 8},
		},
		RequestedRunner: interfaces.RunnerIDCodex,
		RequestedModel:  "gpt-5-codex",
		RequestedProfile: "reviewer",
	})
	if preview.PolicyHash == "" {
		t.Fatal("expected policy hash in preview")
	}
	if preview.MaxChildCount != 8 || preview.MaxConcurrency != 4 {
		t.Fatalf("child/concurrency = %d/%d, want 8/4", preview.MaxChildCount, preview.MaxConcurrency)
	}
	if preview.EffectivePolicy.Mode != jspolicy.ModeReadOnly {
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
		if target.Code == jspolicy.CodeExcessiveMaxAgents {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want excessiveMaxAgents from orchestrator policy validation", targets)
	}
}

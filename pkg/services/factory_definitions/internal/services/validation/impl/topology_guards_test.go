package impl

import (
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestRuleFactoryGuards_InferenceThrottleRequiresModelProvider(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Guards = []factorydefinitions.FactoryGuardConfig{{
		Type:          factorydefinitions.GuardTypeInferenceThrottle,
		RefreshWindow: "15m",
	}}

	findings := ruleFactoryGuards(cfg)
	assertFindingMatch(t, findings, "factory-guard-inference-throttle-model-provider", "guards[0](inference_throttle_guard).modelProvider", "modelProvider")
}

func TestRuleFactoryGuards_InferenceThrottleRejectsInvalidRefreshWindow(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Guards = []factorydefinitions.FactoryGuardConfig{{
		Type:          factorydefinitions.GuardTypeInferenceThrottle,
		ModelProvider: "claude",
		RefreshWindow: "tomorrow",
	}}

	findings := ruleFactoryGuards(cfg)
	assertFindingMatch(t, findings, "factory-guard-inference-throttle-refresh-window", "guards[0](inference_throttle_guard).refreshWindow", "positive duration")
}

func TestRuleFactoryGuards_InferenceThrottleRejectsNonPositiveRefreshWindow(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Guards = []factorydefinitions.FactoryGuardConfig{{
		Type:          factorydefinitions.GuardTypeInferenceThrottle,
		ModelProvider: "claude",
		RefreshWindow: "0s",
	}}

	findings := ruleFactoryGuards(cfg)
	assertFindingMatch(t, findings, "factory-guard-inference-throttle-refresh-window", "guards[0](inference_throttle_guard).refreshWindow", "positive duration")
}

func TestRuleFactoryGuards_RejectsUnsupportedType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Guards = []factorydefinitions.FactoryGuardConfig{{Type: factorydefinitions.GuardTypeVisitCount}}

	findings := ruleFactoryGuards(cfg)
	assertFindingMatch(t, findings, "factory-guard-unknown-type", "guards[0](visit_count)", "factory guards support: inference_throttle_guard")
}

func TestRuleFactoryGuards_ValidInferenceThrottleGuard(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Guards = []factorydefinitions.FactoryGuardConfig{{
		Type:          factorydefinitions.GuardTypeInferenceThrottle,
		ModelProvider: "claude",
		Model:         "claude-sonnet-4-20250514",
		RefreshWindow: "15m",
	}}

	findings := ruleFactoryGuards(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %#v", findings)
	}
}

func TestRuleGuards_VisitCountMissingWorkstation(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:   "ws",
		Guards: []factorydefinitions.GuardConfig{{Type: factorydefinitions.GuardTypeVisitCount, MaxVisits: 3}},
	}}
	findings := ruleGuards(cfg)
	assertFindingExists(t, findings, "guard-visit-count-workstation")
}

func TestRuleGuards_VisitCountInvalidWorkstation(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:   "ws",
		Guards: []factorydefinitions.GuardConfig{{Type: factorydefinitions.GuardTypeVisitCount, Workstation: "nonexistent", MaxVisits: 3}},
	}}
	findings := ruleGuards(cfg)
	assertFindingExists(t, findings, "guard-visit-count-workstation")
}

func TestRuleGuards_VisitCountZeroMaxVisits(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{
		{Name: "ws", Guards: []factorydefinitions.GuardConfig{{Type: factorydefinitions.GuardTypeVisitCount, Workstation: "ws", MaxVisits: 0}}},
	}
	findings := ruleGuards(cfg)
	assertFindingExists(t, findings, "guard-visit-count-max-visits")
}

func TestRuleInvocationBoundLimitsAcceptsNumberStringParameters(t *testing.T) {
	cfg := testBaseConfig()
	cfg.InvocationSignature = &factorydefinitions.InvocationSignatureConfig{Parameters: []factorydefinitions.InvocationParameterConfig{
		{Name: "maxCycles", TypeHint: factorydefinitions.InvocationParameterTypeHintNumberString},
		{Name: "maxTasks", TypeHint: factorydefinitions.InvocationParameterTypeHintNumberString},
	}}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "planner",
		Limits: factorydefinitions.WorkstationLimits{
			MaxGeneratedWorkItems: 9, MaxGeneratedWorkItemsArgument: "maxTasks", MaxGeneratedWorkItemsArgumentOffset: 1,
		},
		Guards: []factorydefinitions.GuardConfig{{
			Type: factorydefinitions.GuardTypeVisitCount, Workstation: "planner", MaxVisits: 8, MaxVisitsArgument: "maxCycles",
		}},
	}}

	if findings := ruleInvocationBoundLimits(cfg); len(findings) != 0 {
		t.Fatalf("ruleInvocationBoundLimits() = %#v, want no findings", findings)
	}
}

func TestRuleInvocationBoundLimitsRejectsMissingOrNonNumericParameters(t *testing.T) {
	cfg := testBaseConfig()
	cfg.InvocationSignature = &factorydefinitions.InvocationSignatureConfig{Parameters: []factorydefinitions.InvocationParameterConfig{
		{Name: "maxTasks", TypeHint: factorydefinitions.InvocationParameterTypeHintString},
	}}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "planner",
		Limits: factorydefinitions.WorkstationLimits{
			MaxGeneratedWorkItems: 9, MaxGeneratedWorkItemsArgument: "maxTasks", MaxGeneratedWorkItemsArgumentOffset: -1,
		},
		Guards: []factorydefinitions.GuardConfig{{
			Type: factorydefinitions.GuardTypeVisitCount, Workstation: "planner", MaxVisits: 8, MaxVisitsArgument: "maxCycles",
		}},
	}}

	findings := ruleInvocationBoundLimits(cfg)
	assertFindingExists(t, findings, "workstation-limit-invocation-bound-parameter")
	assertFindingExists(t, findings, "workstation-limit-invocation-bound-offset")
	assertFindingExists(t, findings, "guard-visit-count-invocation-bound-parameter")
}

func TestRuleGuards_MatchesFieldsMissingMatchConfig(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:   "ws",
		Guards: []factorydefinitions.GuardConfig{{Type: factorydefinitions.GuardTypeMatchesFields}},
	}}

	findings := ruleGuards(cfg)
	if len(findings) != 1 || findings[0].Rule != "guard-matches-fields-input-key" {
		t.Fatalf("expected match_config.input_key finding, got %#v", findings)
	}
}

func TestRuleGuards_MatchesFieldsEmptyInputKey(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Guards: []factorydefinitions.GuardConfig{{
			Type:        factorydefinitions.GuardTypeMatchesFields,
			MatchConfig: &factorydefinitions.GuardMatchConfig{InputKey: "   "},
		}},
	}}

	findings := ruleGuards(cfg)
	if len(findings) != 1 || findings[0].Rule != "guard-matches-fields-input-key" {
		t.Fatalf("expected match_config.input_key finding, got %#v", findings)
	}
}

func TestRuleHostedWorkers_AcceptsHostedLinearWorker(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name:     "linear-poller",
		Type:     factorydefinitions.WorkerTypeHosted,
		Provider: factorydefinitions.HostedWorkerProviderLinear,
		Auth:     &factorydefinitions.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear: &factorydefinitions.HostedLinearWorkerConfig{
			Mapping: factorydefinitions.HostedLinearWorkerMappingConfig{WorkType: "story", State: "init"},
		},
	}}

	findings := ruleHostedWorkers(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %#v", findings)
	}
}

func TestRuleHostedWorkers_RejectsMissingSecretRefAndMapping(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name:     "linear-poller",
		Type:     factorydefinitions.WorkerTypeHosted,
		Provider: factorydefinitions.HostedWorkerProviderLinear,
		Auth:     &factorydefinitions.HostedWorkerAuthConfig{},
		Linear:   &factorydefinitions.HostedLinearWorkerConfig{},
	}}

	findings := ruleHostedWorkers(cfg)
	assertFindingMatch(t, findings, "hosted-worker-auth-secret-ref", "workers[0](linear-poller).auth.secretRef", "auth.secretRef")
	assertFindingMatch(t, findings, "hosted-worker-linear-mapping-work-type", "workers[0](linear-poller).linear.mapping.workType", "mapping.workType")
	assertFindingMatch(t, findings, "hosted-worker-linear-mapping-state", "workers[0](linear-poller).linear.mapping.state", "mapping.state")
}

func TestRuleHostedWorkers_RejectsHostedFieldsOnNonHostedWorker(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name:     "executor",
		Type:     factorydefinitions.WorkerTypeModel,
		Provider: factorydefinitions.HostedWorkerProviderLinear,
		Auth:     &factorydefinitions.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear: &factorydefinitions.HostedLinearWorkerConfig{
			Mapping: factorydefinitions.HostedLinearWorkerMappingConfig{WorkType: "story", State: "init"},
		},
	}}

	findings := ruleHostedWorkers(cfg)
	assertFindingMatch(t, findings, "hosted-worker-provider-unsupported", "workers[0](executor).provider", "cannot declare hosted provider")
	assertFindingMatch(t, findings, "hosted-worker-auth-unsupported", "workers[0](executor).auth", "cannot declare hosted auth")
	assertFindingMatch(t, findings, "hosted-worker-linear-unsupported", "workers[0](executor).linear", "cannot declare hosted LINEAR")
}

func TestRuleGuards_ValidMatchesFieldsGuard(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Guards: []factorydefinitions.GuardConfig{{
			Type:        factorydefinitions.GuardTypeMatchesFields,
			MatchConfig: &factorydefinitions.GuardMatchConfig{InputKey: `.Tags["_last_output"]`},
		}},
	}}

	findings := ruleGuards(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %#v", findings)
	}
}

func TestRuleGuards_UnknownType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:   "ws",
		Guards: []factorydefinitions.GuardConfig{{Type: "bogus"}},
	}}
	findings := ruleGuards(cfg)
	assertFindingExists(t, findings, "guard-unknown-type")
}

func TestRuleGuards_RejectsWorkstationLevelChildFanInTypes(t *testing.T) {
	tests := []struct {
		name      string
		guardType factorydefinitions.GuardType
	}{
		{name: "all children complete", guardType: factorydefinitions.GuardTypeAllChildrenComplete},
		{name: "any child failed", guardType: factorydefinitions.GuardTypeAnyChildFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testBaseConfig()
			cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
				Name:   "ws",
				Guards: []factorydefinitions.GuardConfig{{Type: tt.guardType}},
			}}
			findings := ruleGuards(cfg)
			assertFindingExists(t, findings, "guard-unknown-type")
			if !strings.Contains(findings[0].Message, "use per-input guards for child fan-in") {
				t.Fatalf("expected per-input guard guidance, got %q", findings[0].Message)
			}
		})
	}
}

func TestRuleGuards_ValidGuards(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Guards: []factorydefinitions.GuardConfig{
			{Type: factorydefinitions.GuardTypeVisitCount, Workstation: "ws", MaxVisits: 3},
		},
	}}
	findings := ruleGuards(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

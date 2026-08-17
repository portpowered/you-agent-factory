// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package impl

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func containsAll(value string, substrings ...string) bool {
	for _, substring := range substrings {
		if !strings.Contains(value, substring) {
			return false
		}
	}
	return true
}

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

func TestValidate_ReviewRepeaterContinueTopologyWithBoundedLoopBreaker(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "task",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "in-review", Type: factorydefinitions.StateTypeProcessing},
					{Name: "to-complete", Type: factorydefinitions.StateTypeProcessing},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
					{Name: "failed", Type: factorydefinitions.StateTypeFailed},
				},
			},
			{
				Name: "review",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
					{Name: "fin", Type: factorydefinitions.StateTypeFailed},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "review",
				Kind: factorydefinitions.WorkstationKindRepeater,
				Inputs: []factorydefinitions.IOConfig{
					{WorkTypeName: "task", StateName: "in-review"},
					{WorkTypeName: "review", StateName: "init"},
				},
				Outputs: []factorydefinitions.IOConfig{
					{WorkTypeName: "task", StateName: "to-complete"},
					{WorkTypeName: "review", StateName: "complete"},
				},
				OnContinue: []factorydefinitions.IOConfig{
					{WorkTypeName: "task", StateName: "in-review"},
					{WorkTypeName: "review", StateName: "init"},
				},
				OnRejection: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				OnFailure: []factorydefinitions.IOConfig{
					{WorkTypeName: "task", StateName: "failed"},
					{WorkTypeName: "review", StateName: "fin"},
				},
			},
			{
				Name:    "review-loop-breaker",
				Type:    factorydefinitions.WorkstationTypeLogical,
				Inputs:  []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "in-review"}},
				Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
				Guards: []factorydefinitions.GuardConfig{{
					Type:        factorydefinitions.GuardTypeVisitCount,
					Workstation: "review",
					MaxVisits:   10,
				}},
			},
		},
	}

	result := Validate(cfg)
	if len(result.Targets) != 0 {
		t.Fatalf("review hold topology produced validation targets: %#v", result.Targets)
	}
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

func TestRuleWorkstationKind_UnknownKind(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{Name: "ws", Kind: "bogus"}}
	findings := ruleWorkstationKind(cfg)
	assertFindingExists(t, findings, "workstation-kind")
}

func TestRuleClassifierWorkstations_RejectsNonClassifierWithoutOutputs(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "process-task",
		Type:           factorydefinitions.WorkstationTypeModel,
		WorkerTypeName: "w1",
		Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}}

	findings := ruleClassifierWorkstations(cfg)
	assertFindingExists(t, findings, "workstation-outputs")
}

func TestRuleClassifierWorkstations_AllowsNonClassifierWithoutOnFailureWhenOutputsPresent(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "process-task",
		Type:           factorydefinitions.WorkstationTypeModel,
		WorkerTypeName: "w1",
		Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
	}}

	findings := ruleClassifierWorkstations(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %#v", findings)
	}
}

func TestRuleClassifierWorkstations_RejectsNonClassifierClassificationRoutes(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "process-task",
		Type:           factorydefinitions.WorkstationTypeModel,
		WorkerTypeName: "w1",
		Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{
			{Label: "approved", Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}}},
		},
	}}

	findings := ruleClassifierWorkstations(cfg)
	assertFindingExists(t, findings, "workstation-classification-routes")
}

func TestRuleClassifierWorkstations_RejectsMissingRoutesAndLegacySuccessPaths(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "classify-story",
		Type:           factorydefinitions.WorkstationTypeClassify,
		WorkerTypeName: "w1",
		Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		OnContinue:     []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		OnRejection:    []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
	}}

	findings := ruleClassifierWorkstations(cfg)
	assertFindingExists(t, findings, "classifier-workstation-routes")
	assertFindingExists(t, findings, "classifier-workstation-outputs")
	assertFindingExists(t, findings, "classifier-workstation-on-continue")
	assertFindingExists(t, findings, "classifier-workstation-on-rejection")
}

func TestRuleClassifierWorkstations_RejectsDuplicateLabelsWhitespaceAndEmptyOutputs(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "classify-story",
		Type:           factorydefinitions.WorkstationTypeClassify,
		WorkerTypeName: "w1",
		Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{
			{Label: "approved", Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}}},
			{Label: "approved"},
			{Label: " needs_review ", Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "review"}}},
			{Label: "123", Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "review"}}},
			{Label: "   ", Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}}},
		},
	}}

	findings := ruleClassifierWorkstations(cfg)
	assertFindingExists(t, findings, "classifier-workstation-route-label")
	assertFindingExists(t, findings, "classifier-workstation-route-outputs")
}

func TestRuleClassifierWorkstations_AllowsValidClassifierTopology(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "classify-story",
		Type:           factorydefinitions.WorkstationTypeClassify,
		WorkerTypeName: "w1",
		Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{
			{Label: "approved", Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}}},
			{Label: "needs_review", Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}}},
		},
		OnFailure: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
	}}

	findings := ruleClassifierWorkstations(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRuleClassifierWorkstations_AllowsClassifierWithoutOnFailureWhenRoutesPresent(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "classify-story",
		Type:           factorydefinitions.WorkstationTypeClassify,
		WorkerTypeName: "w1",
		Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{
			{Label: "approved", Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}}},
			{Label: "needs_review", Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}}},
		},
	}}

	findings := ruleClassifierWorkstations(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %#v", findings)
	}
}

func TestRuleCronWorkstations_ValidScheduleCron(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "daily-refresh",
		Kind:           factorydefinitions.WorkstationKindCron,
		WorkerTypeName: "w1",
		Cron: &factorydefinitions.CronConfig{
			Schedule:       "0 * * * *",
			TriggerAtStart: true,
			Jitter:         "30s",
			ExpiryWindow:   "10m",
		},
		Outputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
	}}
	findings := ruleCronWorkstations(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRuleCronWorkstations_ValidRequiredInputCron(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "refresh-ready-task",
		Kind:           factorydefinitions.WorkstationKindCron,
		WorkerTypeName: "w1",
		Cron:           &factorydefinitions.CronConfig{Schedule: "0 * * * *"},
		Inputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
		Outputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task",
			StateName:    "done",
		}},
	}}
	findings := ruleCronWorkstations(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRuleCronWorkstations_MissingCronConfig(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:    "daily-refresh",
		Kind:    factorydefinitions.WorkstationKindCron,
		Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-config")
}

func TestRuleCronWorkstations_MissingSchedule(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:    "daily-refresh",
		Kind:    factorydefinitions.WorkstationKindCron,
		Cron:    &factorydefinitions.CronConfig{},
		Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-schedule")
}

func TestRuleCronWorkstations_InvalidScheduleNamesWorkstationAndValue(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:    "daily-refresh",
		Kind:    factorydefinitions.WorkstationKindCron,
		Cron:    &factorydefinitions.CronConfig{Schedule: "not a cron"},
		Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-schedule")
	if findings[0].Path != "workstations[0](daily-refresh).cron.schedule" {
		t.Fatalf("expected path to name cron workstation and schedule field, got %q", findings[0].Path)
	}
	if !strings.Contains(findings[0].Message, `"not a cron"`) {
		t.Fatalf("expected message to include bad schedule value, got %q", findings[0].Message)
	}
}

func TestRuleCronWorkstations_UnsupportedIntervalNamesWorkstationAndField(t *testing.T) {
	var cron factorydefinitions.CronConfig
	if err := json.Unmarshal([]byte(`{"interval":"5m"}`), &cron); err != nil {
		t.Fatalf("unmarshal cron config: %v", err)
	}
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:    "daily-refresh",
		Kind:    factorydefinitions.WorkstationKindCron,
		Cron:    &cron,
		Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-interval")
	if findings[0].Path != "workstations[0](daily-refresh).cron.interval" {
		t.Fatalf("expected path to name cron workstation and field, got %q", findings[0].Path)
	}
}

func TestRuleCronWorkstations_InvalidJitterNamesWorkstationAndField(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:    "daily-refresh",
		Kind:    factorydefinitions.WorkstationKindCron,
		Cron:    &factorydefinitions.CronConfig{Schedule: "0 * * * *", Jitter: "-1s"},
		Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-jitter")
	if findings[0].Path != "workstations[0](daily-refresh).cron.jitter" {
		t.Fatalf("expected path to name cron workstation and field, got %q", findings[0].Path)
	}
}

func TestRuleCronWorkstations_InvalidExpiryWindowNamesWorkstationAndField(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:    "daily-refresh",
		Kind:    factorydefinitions.WorkstationKindCron,
		Cron:    &factorydefinitions.CronConfig{Schedule: "0 * * * *", ExpiryWindow: "0s"},
		Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-expiry-window")
	if findings[0].Path != "workstations[0](daily-refresh).cron.expiry_window" {
		t.Fatalf("expected path to name cron workstation and field, got %q", findings[0].Path)
	}
}

func TestRuleCronWorkstations_MissingOutput(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "daily-refresh",
		Kind:           factorydefinitions.WorkstationKindCron,
		WorkerTypeName: "w1",
		Cron:           &factorydefinitions.CronConfig{Schedule: "0 * * * *"},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-output")
}

func TestRuleCronWorkstations_ValidLogicalMoveCronWithoutWorker(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "scheduled-route",
		Type: factorydefinitions.WorkstationTypeLogical,
		Kind: factorydefinitions.WorkstationKindCron,
		Cron: &factorydefinitions.CronConfig{Schedule: "0 * * * *"},
		Outputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
	}}
	findings := ruleCronWorkstations(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRuleCronWorkstations_MissingWorker(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "daily-refresh",
		Type: factorydefinitions.WorkstationTypeModel,
		Kind: factorydefinitions.WorkstationKindCron,
		Cron: &factorydefinitions.CronConfig{Schedule: "0 * * * *"},
		Outputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-worker")
	if findings[0].Path != "workstations[0](daily-refresh).worker" {
		t.Fatalf("expected path to name cron workstation and worker field, got %q", findings[0].Path)
	}
}

func TestRuleCronWorkstations_NonCronWithCronConfig(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "processor",
		Kind: factorydefinitions.WorkstationKindStandard,
		Cron: &factorydefinitions.CronConfig{Schedule: "0 * * * *"},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-type")
}

func TestRuleWorkerReferences_NonexistentWorker(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{Name: "ws", WorkerTypeName: "nonexistent"}}
	findings := ruleWorkerReferences(cfg)
	assertFindingExists(t, findings, "workstation-worker-ref")
}

func TestRuleWorkstationKindAndWorker_ValidConfig(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws", Kind: factorydefinitions.WorkstationKindRepeater, WorkerTypeName: "w1",
	}}
	f1 := ruleWorkstationKind(cfg)
	f2 := ruleWorkerReferences(cfg)
	if len(f1)+len(f2) != 0 {
		t.Fatalf("expected no findings, got kind=%v worker=%v", f1, f2)
	}
}

func TestRuleWorkstationKind_AcceptsPoller(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "poller",
		Kind:           factorydefinitions.WorkstationKindPoller,
		WorkerTypeName: "w1",
	}}

	if findings := ruleWorkstationKind(cfg); len(findings) != 0 {
		t.Fatalf("expected no kind findings for poller, got %v", findings)
	}
}

func TestRulePollerWorkstations_RejectsUnsupportedWorkerType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name: "planner",
		Type: factorydefinitions.WorkerTypeModel,
	}}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "linear-poller",
		Kind:           factorydefinitions.WorkstationKindPoller,
		WorkerTypeName: "planner",
	}}

	findings := rulePollerWorkstations(cfg)
	assertFindingExists(t, findings, "poller-worker-type")
	if findings[0].Path != "workstations[0](linear-poller).worker" {
		t.Fatalf("expected path to name poller workstation and worker field, got %q", findings[0].Path)
	}
	if got := findings[0].Message; !containsAll(got, `poller workstation "linear-poller"`, `worker "planner"`, `MODEL_WORKER`, `Automations-owned`) {
		t.Fatalf("expected explicit Automations-owned poller/worker relationship in message, got %q", got)
	}
}

func TestRulePollerWorkstations_AcceptsScriptAndHostedWorkers(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{
		{Name: "script-poller", Type: factorydefinitions.WorkerTypeScript},
		{Name: "hosted-poller", Type: factorydefinitions.WorkerTypeHosted},
	}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{
		{Name: "script", Kind: factorydefinitions.WorkstationKindPoller, WorkerTypeName: "script-poller"},
		{Name: "hosted", Kind: factorydefinitions.WorkstationKindPoller, WorkerTypeName: "hosted-poller"},
	}

	if findings := rulePollerWorkstations(cfg); len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRulePerInputGuards_MissingParentInput(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task", StateName: "init",
			Guard: &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeAllChildrenComplete},
		}},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-parent-input")
}

func TestRulePerInputGuards_ParentInputNotMatching(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task", StateName: "init",
			Guard: &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeAllChildrenComplete, ParentInput: "other"},
		}},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-parent-input")
}

func TestRulePerInputGuards_SelfReference(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task", StateName: "init",
			Guard: &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeAllChildrenComplete, ParentInput: "task"},
		}},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-self-ref")
}

func TestRulePerInputGuards_InvalidSpawnedBy(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = append(cfg.WorkTypes, factorydefinitions.WorkTypeConfig{
		Name: "parent", States: []factorydefinitions.StateConfig{{Name: "init", Type: factorydefinitions.StateTypeInitial}},
	})
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "parent", StateName: "init"},
			{
				WorkTypeName: "task", StateName: "init",
				Guard: &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeAllChildrenComplete, ParentInput: "parent", SpawnedBy: "nonexistent"},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-spawned-by")
}

func TestRulePerInputGuards_UnsupportedType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task", StateName: "init",
			Guard: &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeVisitCount},
		}},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-type")
}

func TestRulePerInputGuards_SameNameMissingMatchInput(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard:        &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeSameName},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-match-input")
}

func TestRulePerInputGuards_SameNameMatchInputNotMatching(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameName,
					MatchInput: "other",
				},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-match-input")
}

func TestRulePerInputGuards_SameNameSelfReference(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameName,
					MatchInput: "task",
				},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-self-ref")
}

func TestRulePerInputGuards_ValidSameNameGuard(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameName,
					MatchInput: "plan",
				},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestConfigValidator_UnsupportedSameNameAllChildrenCompleteJoinArity(t *testing.T) {
	tests := []struct {
		name         string
		inputs       []factorydefinitions.IOConfig
		wantFinding  bool
		wantArity    int
		messageParts []string
	}{
		{
			name: "one input remains accepted",
			inputs: []factorydefinitions.IOConfig{
				{WorkTypeName: "parent", StateName: "ready"},
			},
		},
		{
			name: "two input same-name join remains accepted",
			inputs: []factorydefinitions.IOConfig{
				{WorkTypeName: "parent", StateName: "ready"},
				{
					WorkTypeName: "same",
					StateName:    "ready",
					Guard: &factorydefinitions.InputGuardConfig{
						Type:       factorydefinitions.GuardTypeSameName,
						MatchInput: "parent",
					},
				},
			},
		},
		{
			name: "two input child-complete join remains accepted",
			inputs: []factorydefinitions.IOConfig{
				{WorkTypeName: "parent", StateName: "ready"},
				{
					WorkTypeName: "child",
					StateName:    "complete",
					Guard: &factorydefinitions.InputGuardConfig{
						Type:        factorydefinitions.GuardTypeAllChildrenComplete,
						ParentInput: "parent",
					},
				},
			},
		},
		{
			name: "two input combined join remains accepted",
			inputs: []factorydefinitions.IOConfig{
				{
					WorkTypeName: "parent",
					StateName:    "ready",
					Guard: &factorydefinitions.InputGuardConfig{
						Type:       factorydefinitions.GuardTypeSameName,
						MatchInput: "child",
					},
				},
				{
					WorkTypeName: "child",
					StateName:    "complete",
					Guard: &factorydefinitions.InputGuardConfig{
						Type:        factorydefinitions.GuardTypeAllChildrenComplete,
						ParentInput: "parent",
					},
				},
			},
		},
		{
			name: "three input same-name plus child-complete join is rejected",
			inputs: []factorydefinitions.IOConfig{
				{WorkTypeName: "parent", StateName: "ready"},
				{
					WorkTypeName: "same",
					StateName:    "ready",
					Guard: &factorydefinitions.InputGuardConfig{
						Type:       factorydefinitions.GuardTypeSameName,
						MatchInput: "parent",
					},
				},
				{
					WorkTypeName: "child",
					StateName:    "complete",
					Guard: &factorydefinitions.InputGuardConfig{
						Type:        factorydefinitions.GuardTypeAllChildrenComplete,
						ParentInput: "parent",
					},
				},
			},
			wantFinding: true,
			wantArity:   3,
			messageParts: []string{
				`workstation "fan-in"`,
				"unsupported SAME_NAME plus ALL_CHILDREN_COMPLETE join arity",
				"observed arity is 3 inputs",
				"at most 2 inputs are supported",
				"Split the fan-in into supported two-input workstation stages",
				"reduce the joined inputs",
			},
		},
		{
			name: "four input same-name plus child-complete join is rejected",
			inputs: append([]factorydefinitions.IOConfig{
				{WorkTypeName: "parent", StateName: "ready"},
				{
					WorkTypeName: "same",
					StateName:    "ready",
					Guard: &factorydefinitions.InputGuardConfig{
						Type:       factorydefinitions.GuardTypeSameName,
						MatchInput: "parent",
					},
				},
				{
					WorkTypeName: "child",
					StateName:    "complete",
					Guard: &factorydefinitions.InputGuardConfig{
						Type:        factorydefinitions.GuardTypeAllChildrenComplete,
						ParentInput: "parent",
					},
				},
			}, factorydefinitions.IOConfig{WorkTypeName: "extra", StateName: "ready"}),
			wantFinding: true,
			wantArity:   4,
		},
		{
			name: "three input same-name without child-complete remains accepted",
			inputs: []factorydefinitions.IOConfig{
				{WorkTypeName: "parent", StateName: "ready"},
				{
					WorkTypeName: "same",
					StateName:    "ready",
					Guard: &factorydefinitions.InputGuardConfig{
						Type:       factorydefinitions.GuardTypeSameName,
						MatchInput: "parent",
					},
				},
				{WorkTypeName: "extra", StateName: "ready"},
			},
		},
		{
			name: "three input child-complete without same-name remains accepted",
			inputs: []factorydefinitions.IOConfig{
				{WorkTypeName: "parent", StateName: "ready"},
				{
					WorkTypeName: "child",
					StateName:    "complete",
					Guard: &factorydefinitions.InputGuardConfig{
						Type:        factorydefinitions.GuardTypeAllChildrenComplete,
						ParentInput: "parent",
					},
				},
				{WorkTypeName: "extra", StateName: "ready"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testBaseConfig()
			cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
				Name:   "fan-in",
				Inputs: tt.inputs,
			}}

			result := NewConfigValidator(nil).Validate(cfg)
			var finding *Finding
			for index := range result.Findings {
				if result.Findings[index].Rule == "same-name-all-children-complete-join-arity" {
					finding = &result.Findings[index]
					break
				}
			}

			if !tt.wantFinding {
				if finding != nil {
					t.Fatalf("unexpected unsupported join arity finding: %#v", *finding)
				}
				return
			}
			if finding == nil {
				t.Fatalf("expected unsupported join arity finding, got %#v", result.Findings)
			}
			if finding.Severity != SeverityError {
				t.Fatalf("finding severity = %q, want %q", finding.Severity, SeverityError)
			}
			wantPath := "workstations[0](fan-in).inputs"
			if finding.Path != wantPath {
				t.Fatalf("finding path = %q, want %q", finding.Path, wantPath)
			}
			if tt.messageParts != nil && !containsAll(finding.Message, tt.messageParts...) {
				t.Fatalf("finding message = %q, want parts %v", finding.Message, tt.messageParts)
			}
			if !strings.Contains(finding.Message, fmt.Sprintf("observed arity is %d inputs", tt.wantArity)) {
				t.Fatalf("finding message = %q, want arity %d", finding.Message, tt.wantArity)
			}
		})
	}
}

func TestRulePerInputGuards_SameTraceIDMissingMatchInput(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard:        &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeSameTraceID},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-same-trace-match-input")
}

func TestRulePerInputGuards_SameTraceIDMatchInputNotMatching(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameTraceID,
					MatchInput: "other",
				},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-same-trace-match-input")
}

func TestRulePerInputGuards_SameTraceIDSelfReference(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameTraceID,
					MatchInput: "task",
				},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-same-trace-self-ref")
}

func TestRulePerInputGuards_ValidSameTraceIDGuard(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameTraceID,
					MatchInput: "plan",
				},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRulePerInputGuards_ValidGuard(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = append(cfg.WorkTypes, factorydefinitions.WorkTypeConfig{
		Name: "parent", States: []factorydefinitions.StateConfig{{Name: "init", Type: factorydefinitions.StateTypeInitial}},
	})
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{
		{Name: "spawner"},
		{
			Name: "ws",
			Inputs: []factorydefinitions.IOConfig{
				{WorkTypeName: "parent", StateName: "init"},
				{
					WorkTypeName: "task", StateName: "init",
					Guard: &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeAllChildrenComplete, ParentInput: "parent", SpawnedBy: "spawner"},
				},
			},
		},
	}
	findings := rulePerInputGuards(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestConfigValidator_WorkTypeHandlingBehavior_RejectsMultipleDefaultWorkTypes(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = []factorydefinitions.WorkTypeConfig{
		{Name: "story", States: testStoryStates(), HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault}},
		{Name: "task", States: testStoryStates(), HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault}},
	}

	findings := ruleWorkTypeHandlingBehavior(cfg, false)
	assertFindingMatch(t, findings, "work-type-handling-behavior-unique-default", "factory.workTypes", "expected at most one work type with handlingBehavior DEFAULT")
}

func TestConfigValidator_WorkTypeHandlingBehavior_RequiresDefaultWhenConfigured(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = []factorydefinitions.WorkTypeConfig{{Name: "story", States: testStoryStates()}}

	findings := ruleWorkTypeHandlingBehavior(cfg, true)
	assertFindingMatch(t, findings, "work-type-handling-behavior-required-default", "factory.workTypes", "expected exactly one work type with handlingBehavior DEFAULT")
}

func TestConfigValidator_WorkTypeHandlingBehavior_RejectsUnsupportedValues(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = []factorydefinitions.WorkTypeConfig{{
		Name:             "story",
		States:           testStoryStates(),
		HandlingBehavior: []string{"PROMPT"},
	}}

	findings := ruleWorkTypeHandlingBehavior(cfg, false)
	assertFindingMatch(t, findings, "work-type-handling-behavior-value", `factory.workTypes[0].handlingBehavior[0]`, `unsupported handlingBehavior value "PROMPT"`)
}

func testStoryStates() []factorydefinitions.StateConfig {
	return []factorydefinitions.StateConfig{
		{Name: "init", Type: factorydefinitions.StateTypeInitial},
		{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
	}
}

func TestRuleAgentWorkerTools_RejectsMissingPolicy(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name:       "executor",
		Type:       factorydefinitions.WorkerTypeAgent,
		AgentTools: &factorydefinitions.AgentToolsConfig{},
	}}

	findings := ruleAgentWorkerTools(cfg)

	assertFindingMatch(t, findings, "agent-worker-tools-policy-required", "workers[0](executor).agentTools.policy", "requires an explicit policy")
}

func TestRuleAgentWorkerTools_RejectsUnsupportedPolicy(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name: "executor",
		Type: factorydefinitions.WorkerTypeAgent,
		AgentTools: &factorydefinitions.AgentToolsConfig{
			Policy: "FULL_SHELL",
		},
	}}

	findings := ruleAgentWorkerTools(cfg)

	assertFindingMatch(t, findings, "agent-worker-tools-policy-supported", "workers[0](executor).agentTools.policy", `unsupported agent tool policy "FULL_SHELL"`)
}

func TestRuleAgentWorkerTools_RejectsAgentToolsOnInferenceWorker(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name: "infer",
		Type: factorydefinitions.WorkerTypeInference,
		AgentTools: &factorydefinitions.AgentToolsConfig{
			Policy: factorydefinitions.AgentToolPolicyReadOnly,
		},
	}}

	findings := ruleAgentWorkerTools(cfg)

	assertFindingMatch(t, findings, "agent-worker-tools-worker-type", "workers[0](infer).agentTools", "only supported on AGENT_WORKER")
}

func TestRuleAgentWorkerTools_AllowsAgentWorkerPolicy(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name: "executor",
		Type: factorydefinitions.WorkerTypeAgent,
		AgentTools: &factorydefinitions.AgentToolsConfig{
			Policy: factorydefinitions.AgentToolPolicyEnabled,
		},
	}}

	findings := ruleAgentWorkerTools(cfg)
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestExpectedArtifactTargets_ReportsActionableOwningDefinitionPaths(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "story",
			ExpectedArtifacts: []factorydefinitions.ExpectedArtifactConfig{{
				Name: "", Pattern: "artifacts/story.json",
			}},
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:   "execute-story",
			Inputs: []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			ExpectedArtifacts: []factorydefinitions.ExpectedArtifactConfig{{
				Name: "manifest", Pattern: "artifacts/{{ (index .Inputs 1).Name }}/manifest.json",
			}},
		}},
	}

	targets := ExpectedArtifactTargets(cfg)
	if len(targets) != 2 {
		t.Fatalf("ExpectedArtifactTargets() returned %d targets, want 2: %#v", len(targets), targets)
	}
	workTypeTarget, workstationTarget := targets[0], targets[1]
	if workTypeTarget.Code != CodeWorkTypeInvalidExpectedArtifact ||
		workTypeTarget.Subject.Type != SubjectTypeWorkType || workTypeTarget.Subject.ID != "story" ||
		workTypeTarget.Path != "factory.workTypes[0](story).expectedArtifacts[0]" ||
		!strings.Contains(workTypeTarget.Message, "work type \"story\"") ||
		!strings.Contains(workTypeTarget.Message, "name is required") {
		t.Fatalf("work type target = %#v, want owning definition path and diagnostic", workTypeTarget)
	}
	if workstationTarget.Code != CodeWorkstationInvalidExpectedArtifact ||
		workstationTarget.Subject.Type != SubjectTypeWorkstation || workstationTarget.Subject.ID != "execute-story" ||
		workstationTarget.Path != "factory.workstations[0](execute-story).expectedArtifacts[0]" ||
		!strings.Contains(workstationTarget.Message, "workstation \"execute-story\"") ||
		!strings.Contains(workstationTarget.Message, "cannot be rendered") {
		t.Fatalf("workstation target = %#v, want owning definition path and diagnostic", workstationTarget)
	}
}
func TestWebhookTargetsAcceptsCanonicalFilterAndDeliveryPolicy(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Webhooks = []factorydefinitions.FactoryWebhookConfig{{
		Name:             "monitor",
		Enabled:          true,
		URL:              "https://hooks.example.test/factory",
		SigningSecretRef: "secrets/factory-monitor",
		Filter: factorydefinitions.FactoryWebhookFilterConfig{
			EventTypes:       []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange, factorydefinitions.FactoryWebhookEventTypeDispatchReconciled},
			DispatchStatuses: []string{factorydefinitions.FactoryWebhookDispatchStatusFailed},
		},
		DeliveryPolicy: &factorydefinitions.FactoryWebhookDeliveryPolicyConfig{
			RequestTimeout:    stringPointerForWebhookTest("15s"),
			MaxAttempts:       intPointerForWebhookTest(3),
			InitialBackoff:    stringPointerForWebhookTest("1s"),
			BackoffMultiplier: floatPointerForWebhookTest(1.5),
			MaxBackoff:        stringPointerForWebhookTest("5s"),
		},
	}}

	findings := WebhookTargets(cfg)
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestWebhookTargetsRejectsDispatchFiltersWithNoCompatibleStatus(t *testing.T) {
	tests := []struct {
		name       string
		eventTypes []string
		statuses   []string
	}{
		{
			name:       "response cannot select interrupted",
			eventTypes: []string{factorydefinitions.FactoryWebhookEventTypeDispatchResponse},
			statuses:   []string{factorydefinitions.FactoryWebhookDispatchStatusInterrupted},
		},
		{
			name:       "interrupted cannot select failed",
			eventTypes: []string{factorydefinitions.FactoryWebhookEventTypeDispatchInterrupted},
			statuses:   []string{factorydefinitions.FactoryWebhookDispatchStatusFailed},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testBaseConfig()
			cfg.Webhooks = []factorydefinitions.FactoryWebhookConfig{{
				Name:             "monitor",
				URL:              "https://hooks.example.test/factory",
				SigningSecretRef: "secrets/factory-monitor",
				Filter: factorydefinitions.FactoryWebhookFilterConfig{
					EventTypes:       test.eventTypes,
					DispatchStatuses: test.statuses,
				},
			}}

			findings := WebhookTargets(cfg)
			assertWebhookTargetMatch(
				t,
				findings,
				CodeWebhookDispatchStatusIncompatible,
				"factory.webhooks[0](monitor).filter.dispatchStatuses",
				"no status compatible with the configured dispatch event types",
			)
		})
	}
}

func TestWebhookTargetsAcceptsMixedDispatchFilterStatusCombinations(t *testing.T) {
	tests := []struct {
		name       string
		eventTypes []string
		statuses   []string
	}{
		{
			name: "failed response and interrupted dispatch",
			eventTypes: []string{
				factorydefinitions.FactoryWebhookEventTypeDispatchResponse,
				factorydefinitions.FactoryWebhookEventTypeDispatchInterrupted,
			},
			statuses: []string{
				factorydefinitions.FactoryWebhookDispatchStatusFailed,
				factorydefinitions.FactoryWebhookDispatchStatusInterrupted,
			},
		},
		{
			name: "failed response among mixed work and dispatch events",
			eventTypes: []string{
				factorydefinitions.FactoryWebhookEventTypeWorkStateChange,
				factorydefinitions.FactoryWebhookEventTypeDispatchResponse,
			},
			statuses: []string{factorydefinitions.FactoryWebhookDispatchStatusFailed},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testBaseConfig()
			cfg.Webhooks = []factorydefinitions.FactoryWebhookConfig{{
				Name:             "monitor",
				URL:              "https://hooks.example.test/factory",
				SigningSecretRef: "secrets/factory-monitor",
				Filter: factorydefinitions.FactoryWebhookFilterConfig{
					EventTypes:       test.eventTypes,
					DispatchStatuses: test.statuses,
				},
			}}

			findings := WebhookTargets(cfg)
			if len(findings) != 0 {
				t.Fatalf("findings = %#v, want none", findings)
			}
		})
	}
}

func TestWebhookTargetsRejectsInvalidFieldsWithEndpointPaths(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Webhooks = []factorydefinitions.FactoryWebhookConfig{
		{
			Name:             "monitor",
			URL:              "ftp://hooks.example.test/factory",
			SigningSecretRef: " ",
			Filter: factorydefinitions.FactoryWebhookFilterConfig{
				EventTypes:       []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange, "NOT_CANONICAL"},
				DispatchStatuses: []string{factorydefinitions.FactoryWebhookDispatchStatusFailed},
			},
			DeliveryPolicy: &factorydefinitions.FactoryWebhookDeliveryPolicyConfig{
				MaxAttempts:       intPointerForWebhookTest(0),
				InitialBackoff:    stringPointerForWebhookTest("5s"),
				MaxBackoff:        stringPointerForWebhookTest("1s"),
				BackoffMultiplier: floatPointerForWebhookTest(0.5),
			},
		},
		{
			Name:             " monitor ",
			Enabled:          true,
			URL:              "https://hooks.example.test/other",
			SigningSecretRef: "secrets/other",
			Filter: factorydefinitions.FactoryWebhookFilterConfig{
				EventTypes: []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange},
			},
		},
	}

	findings := WebhookTargets(cfg)
	assertWebhookTargetMatch(t, findings, CodeWebhookURLInvalid, "factory.webhooks[0](monitor).url", "absolute http or https")
	assertWebhookTargetMatch(t, findings, CodeWebhookSecretReferenceRequired, "factory.webhooks[0](monitor).signingSecretRef", "non-empty")
	assertWebhookTargetMatch(t, findings, CodeWebhookEventTypeUnsupported, "factory.webhooks[0](monitor).filter.eventTypes[1]", "NOT_CANONICAL")
	assertWebhookTargetMatch(t, findings, CodeWebhookDeliveryPolicyInvalid, "factory.webhooks[0](monitor).deliveryPolicy.maxAttempts", "positive")
	assertWebhookTargetMatch(t, findings, CodeWebhookDeliveryPolicyInvalid, "factory.webhooks[0](monitor).deliveryPolicy.backoffMultiplier", "at least 1")
	assertWebhookTargetMatch(t, findings, CodeWebhookDeliveryPolicyInvalid, "factory.webhooks[0](monitor).deliveryPolicy.maxBackoff", "initialBackoff")
	assertWebhookTargetMatch(t, findings, CodeWebhookNameDuplicate, "factory.webhooks[1]( monitor ).name", "duplicates")
}

func assertWebhookTargetMatch(t *testing.T, targets []Target, code, pathSubstring, messageSubstring string) {
	t.Helper()
	for _, target := range targets {
		if target.Code != code || target.Severity != SeverityError || !strings.Contains(target.Path, pathSubstring) {
			continue
		}
		if !strings.Contains(target.Message, messageSubstring) {
			t.Fatalf("target message = %q, want substring %q", target.Message, messageSubstring)
		}
		return
	}
	t.Fatalf("expected error target with code %q, got %v", code, targets)
}

func stringPointerForWebhookTest(value string) *string { return &value }

func intPointerForWebhookTest(value int) *int { return &value }

func floatPointerForWebhookTest(value float64) *float64 { return &value }

func TestValidateHumanApprovalWorkstation(t *testing.T) {
	cfg := humanApprovalTestFactory()
	result := Validate(cfg)
	if len(result.Targets) != 0 {
		t.Fatalf("valid HUMAN_APPROVAL definition produced validation targets: %+v", result.Targets)
	}
}

func TestValidateHumanApprovalWorkstationReportsFieldSpecificDiagnostics(t *testing.T) {
	cfg := humanApprovalTestFactory()
	workstation := &cfg.Workstations[0]
	workstation.ID = ""
	workstation.Description = nil
	workstation.Inputs = nil
	workstation.Outputs = nil
	workstation.OnRejection = nil
	workstation.WorkerTypeName = "fake-worker"
	workstation.Runner = "codex"
	workstation.Operation = "TTS"
	workstation.PromptFile = "prompt.md"
	workstation.OutputSchema = `{ "type": "object" }`
	workstation.Timeout = "1m"
	workstation.Env = map[string]string{"SECRET": "value"}
	workstation.Worktree = "worktree"
	workstation.Resources = []factorydefinitions.ResourceConfig{{Name: "slot", Capacity: 1}}
	workstation.ClassificationRoutes = []factorydefinitions.ClassificationRouteConfig{{Label: "approve"}}
	workstation.OnContinue = []factorydefinitions.IOConfig{{WorkTypeName: "review", StateName: "pending"}}
	workstation.OnFailure = []factorydefinitions.IOConfig{{WorkTypeName: "review", StateName: "failed"}}

	targets := humanApprovalWorkstationTargets(cfg)
	wantPaths := []string{
		"factory.workstations[0](review).id", "factory.workstations[0](review).description",
		"factory.workstations[0](review).inputs", "factory.workstations[0](review).outputs",
		"factory.workstations[0](review).onRejection", "factory.workstations[0](review).worker",
		"factory.workstations[0](review).runner", "factory.workstations[0](review).operation",
		"factory.workstations[0](review).promptFile", "factory.workstations[0](review).outputSchema",
		"factory.workstations[0](review).timeout", "factory.workstations[0](review).env",
		"factory.workstations[0](review).worktree", "factory.workstations[0](review).resources",
		"factory.workstations[0](review).classification_routes", "factory.workstations[0](review).onContinue",
		"factory.workstations[0](review).onFailure",
	}
	got := make(map[string]bool, len(targets))
	for _, target := range targets {
		got[target.Path] = true
	}
	for _, path := range wantPaths {
		if !got[path] {
			t.Errorf("missing field-specific HUMAN_APPROVAL diagnostic for %q", path)
		}
	}
}

func TestValidateHumanApprovalWorkstationRejectsJavaScriptFactory(t *testing.T) {
	cfg := humanApprovalTestFactory()
	cfg.Orchestrator = &factorydefinitions.FactoryOrchestratorConfig{
		Kind: factorydefinitions.OrchestratorKindJavaScript,
		JavaScript: &factorydefinitions.FactoryOrchestratorJavaScriptConfig{
			InlineSource: &factorydefinitions.FactoryOrchestratorJavaScriptInlineSource{Inline: "export default {}"},
		},
	}
	for _, target := range humanApprovalWorkstationTargets(cfg) {
		if target.Path == "factory.workstations[0](review).type" {
			return
		}
	}
	t.Fatal("expected JavaScript-specific HUMAN_APPROVAL diagnostic")
}

func humanApprovalTestFactory() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		Name: "approval-factory",
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "review",
			States: []factorydefinitions.StateConfig{
				{Name: "pending", Type: factorydefinitions.StateTypeInitial},
				{Name: "approved", Type: factorydefinitions.StateTypeTerminal},
				{Name: "rejected", Type: factorydefinitions.StateTypeProcessing},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			ID: "review-approval", Name: "review", Type: factorydefinitions.WorkstationTypeHumanApproval,
			Description: &factorydefinitions.NameValueConfig{Type: factorydefinitions.NameValueTypeLocalizableAsset, Value: "A human reviews the work."},
			Inputs:      []factorydefinitions.IOConfig{{WorkTypeName: "review", StateName: "pending"}},
			Outputs:     []factorydefinitions.IOConfig{{WorkTypeName: "review", StateName: "approved"}},
			OnRejection: []factorydefinitions.IOConfig{{WorkTypeName: "review", StateName: "rejected"}},
		}},
	}
}

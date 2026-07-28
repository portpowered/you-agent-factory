package impl

import (
	"encoding/json"
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
	if got := findings[0].Message; !containsAll(got, `poller workstation "linear-poller"`, `worker "planner"`, `MODEL_WORKER`) {
		t.Fatalf("expected explicit poller/worker relationship in message, got %q", got)
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

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

package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBuiltInFactoryJSON_LoadsReviewGatedTopology(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Name != PackagedFactoryName || cfg.Project != PackagedFactoryProject {
		t.Fatalf("identity = %q/%q", cfg.Name, cfg.Project)
	}
	if cfg.InvocationReturn == nil || cfg.InvocationReturn.WorkTypeName != PackagedWorkTypeName || cfg.InvocationReturn.TerminalState != PackagedInvocationReturnTerminalState {
		t.Fatalf("invocationReturn = %#v", cfg.InvocationReturn)
	}
	if len(cfg.Workstations) != 2 || len(cfg.Workers) != 2 {
		t.Fatalf("topology = %#v", cfg)
	}
	execute, executeOK := workstationByName(cfg.Workstations, PackagedExecuteWorkstationName)
	review, reviewOK := workstationByName(cfg.Workstations, PackagedReviewWorkstationName)
	if !executeOK || !reviewOK {
		t.Fatal("missing execute or review workstation")
	}
	if execute.WorkPropagation == nil || execute.WorkPropagation.Mode != interfaces.WorkPropagationModePreserveInput {
		t.Fatalf("execute work propagation = %#v, want PRESERVE_INPUT", execute.WorkPropagation)
	}
	if execute.Outputs[0].StateName != "in-review" || review.Outputs[0].StateName != "approved" || review.OnRejection[0].StateName != "init" || review.OnFailure[0].StateName != "failed" {
		t.Fatalf("review routes = %#v", review)
	}
	if review.OutcomeFormat != interfaces.WorkstationOutcomeFormatDecisionEnvelope {
		t.Fatalf("outcomeFormat = %q", review.OutcomeFormat)
	}
	for _, target := range factoryvalidation.Validate(cfg).Targets {
		if target.Severity == factoryvalidation.SeverityError {
			t.Fatalf("validation target = %#v", target)
		}
	}
}

func TestMaterializedFactory_RetainsReviewGate(t *testing.T) {
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	review, ok := workstationByName(loaded.FactoryConfig().Workstations, PackagedReviewWorkstationName)
	if !ok || len(review.OnRejection) != 1 || review.OnRejection[0].StateName != "init" {
		t.Fatalf("materialized review = %#v", review)
	}
}

func TestMaterializedFactory_AcceptsWorkerModelConfigurationWithoutWeakeningReviewGate(t *testing.T) {
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	setMaterializedWorkerModel(t, dir, "review-work-executor", "CODEX", "gpt-5-codex")
	setMaterializedWorkerModel(t, dir, "review-work-reviewer", "CODEX", "gpt-5-codex")

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	for _, workerName := range []string{"review-work-executor", "review-work-reviewer"} {
		worker, ok := loaded.Worker(workerName)
		if !ok || worker.ModelProvider != "codex" || worker.Model != "gpt-5-codex" {
			t.Fatalf("worker %q = %#v, want configured codex model", workerName, worker)
		}
	}
	review, ok := workstationByName(loaded.FactoryConfig().Workstations, PackagedReviewWorkstationName)
	if !ok || review.OutcomeFormat != interfaces.WorkstationOutcomeFormatDecisionEnvelope || len(review.OnRejection) != 1 || review.OnRejection[0].StateName != "init" {
		t.Fatalf("configured review gate = %#v, want mandatory decision-envelope rejection loop", review)
	}
}

func TestBuiltInFactoryJSON_RejectsUnsupportedWorkerModelProvider(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	cfg.Workers[0].ModelProvider = "UNSUPPORTED"
	result := factoryvalidation.Validate(cfg)
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeWorkerUnsupportedModelProvider {
			return
		}
	}
	t.Fatalf("factory validation accepted unsupported worker model provider: targets=%#v", result.Targets)
}

func setMaterializedWorkerModel(t *testing.T, factoryDir, workerName, provider, model string) {
	t.Helper()
	path := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	var factory map[string]any
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("Unmarshal factory.json: %v", err)
	}
	workers, ok := factory["workers"].([]any)
	if !ok {
		t.Fatal("factory workers missing")
	}
	for _, entry := range workers {
		worker, ok := entry.(map[string]any)
		if !ok || worker["name"] != workerName {
			continue
		}
		worker["modelProvider"] = provider
		worker["model"] = model
		updated, err := json.MarshalIndent(factory, "", "  ")
		if err != nil {
			t.Fatalf("Marshal factory.json: %v", err)
		}
		if err := os.WriteFile(path, updated, 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", path, err)
		}
		return
	}
	t.Fatalf("worker %q not found", workerName)
}

func workstationByName(workstations []interfaces.FactoryWorkstationConfig, name string) (interfaces.FactoryWorkstationConfig, bool) {
	for _, workstation := range workstations {
		if workstation.Name == name {
			return workstation, true
		}
	}
	return interfaces.FactoryWorkstationConfig{}, false
}

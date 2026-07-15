package topologytests

import (
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

func topologyTestBaseConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []workerconfig.Config{{Name: "w1"}},
	}
}

func TestCanonicalStructuralFindings_InvalidInputPlaceReference(t *testing.T) {
	cfg := topologyTestBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:   "ws",
		Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
	}}

	findings := factoryconfig.CanonicalStructuralFindings(cfg)
	assertCanonicalFindingCode(t, findings, factoryvalidation.CodeDanglingPlaceReference)
	assertCanonicalFindingPath(t, findings, "factory.workstations[0].inputs[0]")
	assertCanonicalTargetSubject(t, cfg, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeRoute,
		ID:       "ws->task:bogus",
		Location: factoryvalidation.SubjectLocationInputs,
	})
}

func TestCanonicalStructuralFindings_InvalidOutputPlaceReference(t *testing.T) {
	cfg := topologyTestBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:    "ws",
		Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
	}}

	findings := factoryconfig.CanonicalStructuralFindings(cfg)
	assertCanonicalFindingCode(t, findings, factoryvalidation.CodeDanglingPlaceReference)
	assertCanonicalFindingPath(t, findings, "factory.workstations[0].outputs[0]")
	assertCanonicalTargetSubject(t, cfg, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeRoute,
		ID:       "ws->task:bogus",
		Location: factoryvalidation.SubjectLocationOutputs,
	})
}

func TestCanonicalStructuralFindings_InvalidOnRejectionPlaceReference(t *testing.T) {
	cfg := topologyTestBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:        "ws",
		Inputs:      []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
	}}

	findings := factoryconfig.CanonicalStructuralFindings(cfg)
	assertCanonicalFindingCode(t, findings, factoryvalidation.CodeDanglingPlaceReference)
	assertCanonicalFindingPath(t, findings, "factory.workstations[0].onRejection[0]")
	assertCanonicalTargetSubject(t, cfg, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeRoute,
		ID:       "ws->task:bogus",
		Location: factoryvalidation.SubjectLocationOnRejection,
	})
}

func TestCanonicalStructuralFindings_InvalidOnFailurePlaceReference(t *testing.T) {
	cfg := topologyTestBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:      "ws",
		Inputs:    []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		OnFailure: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
	}}

	findings := factoryconfig.CanonicalStructuralFindings(cfg)
	assertCanonicalFindingCode(t, findings, factoryvalidation.CodeDanglingPlaceReference)
	assertCanonicalFindingPath(t, findings, "factory.workstations[0].onFailure[0]")
	assertCanonicalTargetSubject(t, cfg, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeRoute,
		ID:       "ws->task:bogus",
		Location: factoryvalidation.SubjectLocationOnFailure,
	})
}

func TestCanonicalStructuralFindings_ValidPlaceReferences(t *testing.T) {
	cfg := topologyTestBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:        "ws",
		Inputs:      []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:     []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		OnFailure:   []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
	}}

	findings := factoryconfig.CanonicalStructuralFindings(cfg)
	for _, finding := range findings {
		if finding.Rule == factoryvalidation.CodeDanglingPlaceReference {
			t.Fatalf("unexpected dangling place finding: %#v", finding)
		}
	}
}

func TestCanonicalStructuralFindings_InvalidClassificationRouteOutput(t *testing.T) {
	cfg := topologyTestBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "classifier",
		Type:           interfaces.WorkstationTypeClassify,
		WorkerTypeName: "w1",
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		ClassificationRoutes: []interfaces.ClassificationRouteConfig{{
			Label:   "approved",
			Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
		}},
	}}

	findings := factoryconfig.CanonicalStructuralFindings(cfg)
	assertCanonicalFindingCode(t, findings, factoryvalidation.CodeDanglingPlaceReference)
	assertCanonicalFindingPath(t, findings, "factory.workstations[0].classificationRoutes[0].outputs[0]")
}

func TestConfigValidator_PreservesOperationalAndStructuralValidationCoverage(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		InputTypes: []interfaces.InputTypeConfig{{Name: "default", Type: interfaces.InputKindDefault}},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{{
				Name: "init",
				Type: interfaces.StateTypeInitial,
			}},
		}},
		Workers: []workerconfig.Config{
			{Name: "w1"},
			{Name: "planner", Type: interfaces.WorkerTypeModel},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "classifier",
				Type:           interfaces.WorkstationTypeClassify,
				Kind:           "bogus-kind",
				WorkerTypeName: "missing-worker",
				Inputs: []interfaces.IOConfig{{
					WorkTypeName: "task",
					StateName:    "init",
					Guard:        &interfaces.InputGuardConfig{Type: interfaces.GuardTypeAllChildrenComplete},
				}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "missing-state"}},
				Guards: []interfaces.GuardConfig{{
					Type: interfaces.GuardTypeVisitCount,
				}},
			},
			{
				Name:    "daily-refresh",
				Kind:    interfaces.WorkstationKindCron,
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			},
			{
				Name:           "linear-poller",
				Kind:           interfaces.WorkstationKindPoller,
				WorkerTypeName: "planner",
			},
			{
				Name:           "repeater-loop",
				Kind:           interfaces.WorkstationKindRepeater,
				WorkerTypeName: "w1",
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			},
		},
		Resources: []factoryresource.Config{{
			Name: "quota",
			Type: factoryresource.TypeProviderQuota,
		}},
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			RequiredTools: []interfaces.RequiredToolConfig{{Name: "", Command: ""}},
			BundledFiles: []interfaces.BundledFileConfig{{
				Type: interfaces.BundledFileTypeScript,
			}},
		},
	}

	result := factoryconfig.NewConfigValidator().Validate(cfg)
	if !result.HasErrors() {
		t.Fatal("expected validation errors")
	}

	assertFindingExists(t, result.Findings, "input-type-reserved")
	assertFindingExists(t, result.Findings, factoryvalidation.CodeDanglingPlaceReference)
	assertFindingExists(t, result.Findings, factoryvalidation.CodeDanglingWorkerReference)
	assertFindingExists(t, result.Findings, factoryvalidation.CodeWorkstationMissingRejectionRoute)
	assertFindingExists(t, result.Findings, "workstation-kind")
	assertFindingExists(t, result.Findings, "classifier-workstation-outputs")
	assertFindingExists(t, result.Findings, "guard-visit-count-workstation")
	assertFindingExists(t, result.Findings, "per-input-guard-parent-input")
	assertFindingExists(t, result.Findings, "cron-config")
	assertFindingExists(t, result.Findings, "poller-worker-type")
	assertFindingExists(t, result.Findings, "resource-provider-quota-model")
	assertFindingExists(t, result.Findings, "required-tool-name")
	assertFindingExists(t, result.Findings, "bundled-file-target-path")
}

func TestCanonicalStructuralFindings_RejectsUnsupportedManagedRuntimeIdentity(t *testing.T) {
	cfg := topologyTestBaseConfig()
	cfg.Resources = []factoryresource.Config{{
		Name:       "unknown-cache",
		Type:       factoryresource.TypeModel,
		Capacity:   1,
		Model:      "UNKNOWN_RUNTIME",
		Backend:    "LLAMACPP",
		LoadPolicy: "ON_DEMAND",
	}}

	findings := factoryconfig.CanonicalStructuralFindings(cfg)
	assertFindingExists(t, findings, factoryvalidation.CodeManagedRuntimeUnsupportedIdentity)
}

func TestCanonicalStructuralFindings_RejectsLocalWorkerWithoutModelResource(t *testing.T) {
	cfg := topologyTestBaseConfig()
	cfg.Workers = []workerconfig.Config{{
		Name:          "voice-local",
		Type:          interfaces.WorkerTypeModel,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: workerconfig.ModelLocalityLocal,
	}}

	findings := factoryconfig.CanonicalStructuralFindings(cfg)
	assertFindingExists(t, findings, factoryvalidation.CodeManagedRuntimeWorkerMissingDep)
}

func TestCanonicalStructuralFindings_RejectsUnsupportedWorkPropagationMode(t *testing.T) {
	cfg := topologyTestBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:   "process",
		Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs: []interfaces.IOConfig{
			{WorkTypeName: "task", StateName: "done"},
		},
		WorkPropagation: &interfaces.WorkPropagationConfig{
			Mode: interfaces.WorkPropagationMode("MERGE_PAYLOAD"),
		},
	}}

	findings := factoryconfig.CanonicalStructuralFindings(cfg)
	assertCanonicalFindingCode(t, findings, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode)
	assertCanonicalFindingPath(t, findings, "factory.workstations[0](process).workPropagation.mode")
}

func assertCanonicalFindingCode(t *testing.T, findings []factoryconfig.Finding, code string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Rule == code && finding.Severity == factoryconfig.SeverityError {
			return
		}
	}
	t.Fatalf("expected canonical finding code %q, got %#v", code, findings)
}

func assertCanonicalFindingPath(t *testing.T, findings []factoryconfig.Finding, path string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Path == path {
			return
		}
	}
	t.Fatalf("expected canonical finding path %q, got %#v", path, findings)
}

func assertCanonicalTargetSubject(t *testing.T, cfg *interfaces.FactoryConfig, want factoryvalidation.Subject) {
	t.Helper()
	for _, target := range factoryvalidation.Validate(cfg).Targets {
		if target.Subject == want {
			return
		}
	}
	t.Fatalf("expected canonical subject %#v, got targets %#v", want, factoryvalidation.Validate(cfg).Targets)
}

func assertFindingExists(t *testing.T, findings []factoryconfig.Finding, rule string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Rule == rule && finding.Severity == factoryconfig.SeverityError {
			return
		}
	}
	t.Fatalf("expected error finding with rule %q, got %v", rule, findings)
}

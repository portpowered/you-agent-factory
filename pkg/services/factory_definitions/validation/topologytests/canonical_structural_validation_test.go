package topologytests

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

func canonicalTopologyFindings(
	t *testing.T,
	cfg *factorydefinitions.FactoryConfig,
) []factorydefinitions.TopologyFinding {
	t.Helper()
	return factoryvalidation.New(nil).
		ValidateTopology(t.Context(), cfg, nil).
		Findings
}

func topologyTestBaseConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "done", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "w1"}},
	}
}

func TestCanonicalStructuralFindings_InvalidInputPlaceReference(t *testing.T) {
	cfg := topologyTestBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:   "ws",
		Inputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
	}}

	findings := canonicalTopologyFindings(t, cfg)
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
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:    "ws",
		Inputs:  []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
	}}

	findings := canonicalTopologyFindings(t, cfg)
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
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:        "ws",
		Inputs:      []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		OnRejection: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
	}}

	findings := canonicalTopologyFindings(t, cfg)
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
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:      "ws",
		Inputs:    []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		OnFailure: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
	}}

	findings := canonicalTopologyFindings(t, cfg)
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
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:        "ws",
		Inputs:      []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:     []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		OnRejection: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		OnFailure:   []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
	}}

	findings := canonicalTopologyFindings(t, cfg)
	for _, finding := range findings {
		if finding.Rule == factoryvalidation.CodeDanglingPlaceReference {
			t.Fatalf("unexpected dangling place finding: %#v", finding)
		}
	}
}

func TestCanonicalStructuralFindings_InvalidClassificationRouteOutput(t *testing.T) {
	cfg := topologyTestBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "classifier",
		Type:           factorydefinitions.WorkstationTypeClassify,
		WorkerTypeName: "w1",
		Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{{
			Label:   "approved",
			Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
		}},
	}}

	findings := canonicalTopologyFindings(t, cfg)
	assertCanonicalFindingCode(t, findings, factoryvalidation.CodeDanglingPlaceReference)
	assertCanonicalFindingPath(t, findings, "factory.workstations[0].classificationRoutes[0].outputs[0]")
}

func TestConfigValidator_PreservesOperationalAndStructuralValidationCoverage(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{
		InputTypes: []factorydefinitions.InputTypeConfig{{Name: "default", Type: factorydefinitions.InputKindDefault}},
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{{
				Name: "init",
				Type: factorydefinitions.StateTypeInitial,
			}},
		}},
		Workers: []factorydefinitions.FactoryWorkerConfig{
			{Name: "w1"},
			{Name: "planner", Type: factorydefinitions.WorkerTypeModel},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name:           "classifier",
				Type:           factorydefinitions.WorkstationTypeClassify,
				Kind:           "bogus-kind",
				WorkerTypeName: "missing-worker",
				Inputs: []factorydefinitions.IOConfig{{
					WorkTypeName: "task",
					StateName:    "init",
					Guard:        &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeAllChildrenComplete},
				}},
				Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "missing-state"}},
				Guards: []factorydefinitions.GuardConfig{{
					Type: factorydefinitions.GuardTypeVisitCount,
				}},
			},
			{
				Name:    "daily-refresh",
				Kind:    factorydefinitions.WorkstationKindCron,
				Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			},
			{
				Name:           "linear-poller",
				Kind:           factorydefinitions.WorkstationKindPoller,
				WorkerTypeName: "planner",
			},
			{
				Name:           "repeater-loop",
				Kind:           factorydefinitions.WorkstationKindRepeater,
				WorkerTypeName: "w1",
				Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			},
		},
		Resources: []factorydefinitions.ResourceConfig{{
			Name: "quota",
			Type: factorydefinitions.ResourceTypeProviderQuota,
		}},
		ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
			RequiredTools: []factorydefinitions.RequiredToolConfig{{Name: "", Command: ""}},
			BundledFiles: []factorydefinitions.BundledFileConfig{{
				Type: factorydefinitions.BundledFileTypeScript,
			}},
		},
	}

	result := factoryvalidation.New(nil).ValidateTopology(t.Context(), cfg, nil)
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
	cfg.Resources = []factorydefinitions.ResourceConfig{{
		Name:       "unknown-cache",
		Type:       factorydefinitions.ResourceTypeModel,
		Capacity:   1,
		Model:      "UNKNOWN_RUNTIME",
		Backend:    "LLAMACPP",
		LoadPolicy: "ON_DEMAND",
	}}

	findings := canonicalTopologyFindings(t, cfg)
	assertFindingExists(t, findings, factoryvalidation.CodeManagedRuntimeUnsupportedIdentity)
}

func TestCanonicalStructuralFindings_RejectsLocalWorkerWithoutModelResource(t *testing.T) {
	cfg := topologyTestBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name:          "voice-local",
		Type:          factorydefinitions.WorkerTypeModel,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: factorydefinitions.ModelLocalityLocal,
	}}

	findings := canonicalTopologyFindings(t, cfg)
	assertFindingExists(t, findings, factoryvalidation.CodeManagedRuntimeWorkerMissingDep)
}

func TestCanonicalStructuralFindings_RejectsUnsupportedWorkPropagationMode(t *testing.T) {
	cfg := topologyTestBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:   "process",
		Inputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "task", StateName: "done"},
		},
		WorkPropagation: &factorydefinitions.WorkPropagationConfig{
			Mode: factorydefinitions.WorkPropagationMode("MERGE_PAYLOAD"),
		},
	}}

	findings := canonicalTopologyFindings(t, cfg)
	assertCanonicalFindingCode(t, findings, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode)
	assertCanonicalFindingPath(t, findings, "factory.workstations[0](process).workPropagation.mode")
}

func assertCanonicalFindingCode(t *testing.T, findings []factorydefinitions.TopologyFinding, code string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Rule == code && finding.Severity == factorydefinitions.ValidationSeverityError {
			return
		}
	}
	t.Fatalf("expected canonical finding code %q, got %#v", code, findings)
}

func assertCanonicalFindingPath(t *testing.T, findings []factorydefinitions.TopologyFinding, path string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Path == path {
			return
		}
	}
	t.Fatalf("expected canonical finding path %q, got %#v", path, findings)
}

func assertCanonicalTargetSubject(t *testing.T, cfg *factorydefinitions.FactoryConfig, want factoryvalidation.Subject) {
	t.Helper()
	for _, target := range factoryvalidation.Validate(cfg).Targets {
		if target.Subject == want {
			return
		}
	}
	t.Fatalf("expected canonical subject %#v, got targets %#v", want, factoryvalidation.Validate(cfg).Targets)
}

func assertFindingExists(t *testing.T, findings []factorydefinitions.TopologyFinding, rule string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Rule == rule && finding.Severity == factorydefinitions.ValidationSeverityError {
			return
		}
	}
	t.Fatalf("expected error finding with rule %q, got %v", rule, findings)
}

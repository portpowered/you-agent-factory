package validation_test

import (
	"errors"
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type stubRequiredToolChecker map[string]factorydefinitions.RequiredToolCheckResult

func (s stubRequiredToolChecker) Check(tool factorydefinitions.RequiredToolConfig) factorydefinitions.RequiredToolCheckResult {
	if result, ok := s[tool.Command]; ok {
		return result
	}
	return factorydefinitions.RequiredToolCheckResult{}
}

func validPetriFactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		Name: "transitional-export-validation",
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "done", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workers: []workerconfig.Config{{Name: "worker-a"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker-a",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			OnFailure:      []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		}},
	}
}

func TestValidateGraphTopology_NilOrNonPetriConfigReturnsEmptyResult(t *testing.T) {
	t.Parallel()

	if result := factoryvalidation.ValidateGraphTopology(nil); result.HasBlockingTargets() {
		t.Fatalf("nil config targets = %#v, want none", result.Targets)
	}
	if result := factoryvalidation.ValidateGraphTopology(&factorydefinitions.FactoryConfig{Name: "alpha"}); result.HasBlockingTargets() {
		t.Fatalf("non-petri config targets = %#v, want none", result.Targets)
	}
}

func TestValidateDeclarativeRequiredTools_ExportedHelperMapsManifestShapeDefects(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		Name: "required-tool-shape",
		ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
			RequiredTools: []factorydefinitions.RequiredToolConfig{{}},
		},
	}

	result := factoryvalidation.ValidateDeclarativeRequiredTools(cfg, nil)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking required-tool targets")
	}
	codes := make(map[string]struct{}, len(result.Targets))
	for _, target := range result.Targets {
		codes[target.Code] = struct{}{}
	}
	for _, want := range []string{
		factoryvalidation.CodeRequiredToolName,
		factoryvalidation.CodeRequiredToolCommand,
	} {
		if _, ok := codes[want]; !ok {
			t.Fatalf("targets = %#v, want code %q", result.Targets, want)
		}
	}
}

func TestValidateDeclarativeRequiredTools_ExportedHelperMapsVersionProbeFailure(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		Name: "required-tool-probe",
		ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
			RequiredTools: []factorydefinitions.RequiredToolConfig{{
				Name:        "Versioned helper",
				Command:     "versioned-tool",
				VersionArgs: []string{"--version"},
			}},
		},
	}
	checker := stubRequiredToolChecker{
		"versioned-tool": {
			FailureKind: factorydefinitions.RequiredToolFailureKindVersionProbe,
			Err:         errors.New(`required tool "Versioned helper" version probe failed`),
		},
	}

	result := factoryvalidation.ValidateDeclarativeRequiredTools(cfg, checker)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking required-tool targets")
	}
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeRequiredToolVersionProbe {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want version probe required-tool target", result.Targets)
	}
}

func TestValidateDeclarativeRequiredTools_ExportedHelperReturnsEmptyWhenCheckerSucceeds(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		Name: "required-tool-success",
		ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
			RequiredTools: []factorydefinitions.RequiredToolConfig{{
				Name:    "Present helper",
				Command: "present-tool",
			}},
		},
	}
	checker := stubRequiredToolChecker{"present-tool": {}}

	result := factoryvalidation.ValidateDeclarativeRequiredTools(cfg, checker)
	if result.HasBlockingTargets() {
		t.Fatalf("targets = %#v, want none", result.Targets)
	}
}

func TestValidateGraphTopology_ExportedHelperReturnsTypedDanglingPlaceTarget(t *testing.T) {
	t.Parallel()

	cfg := validPetriFactoryConfig()
	cfg.Workstations[0].Outputs = []factorydefinitions.IOConfig{{
		WorkTypeName: "task",
		StateName:    "bogus",
	}}

	result := factoryvalidation.ValidateGraphTopology(cfg)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking topology targets")
	}
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeDanglingPlaceReference {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want dangling place topology target", result.Targets)
	}
}

func TestValidateDeclarativeRequiredTools_ExportedHelperMapsVersionArgsDefect(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		Name: "required-tool-version-args",
		ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
			RequiredTools: []factorydefinitions.RequiredToolConfig{{
				Name:        "Python",
				Command:     "python",
				VersionArgs: []string{"--version", ""},
			}},
		},
	}

	result := factoryvalidation.ValidateDeclarativeRequiredTools(cfg, nil)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking required-tool targets")
	}
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeRequiredToolVersionArgs {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want version-args required-tool target", result.Targets)
	}
}

func TestValidateDeclarativeRequiredTools_ExportedHelperUsesNamedSubjectID(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		Name: "required-tool-named-subject",
		ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
			RequiredTools: []factorydefinitions.RequiredToolConfig{{
				Name:    "Named helper",
				Command: "missing-tool",
			}},
		},
	}
	checker := stubRequiredToolChecker{
		"missing-tool": {
			FailureKind: factorydefinitions.RequiredToolFailureKindMissing,
			Err:         errors.New(`required tool "Named helper" command "missing-tool" was not found on PATH`),
		},
	}

	result := factoryvalidation.ValidateDeclarativeRequiredTools(cfg, checker)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking required-tool targets")
	}
	for _, target := range result.Targets {
		if target.Subject.ID == "Named helper" {
			return
		}
	}
	t.Fatalf("targets = %#v, want named required-tool subject id", result.Targets)
}

func TestValidateDeclarativeRequiredTools_ExportedHelperReturnsTypedMissingTarget(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		Name: "required-tool-export",
		ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
			RequiredTools: []factorydefinitions.RequiredToolConfig{{
				Name:    "Missing helper",
				Command: "missing-tool",
			}},
		},
	}
	checker := stubRequiredToolChecker{
		"missing-tool": {
			FailureKind: factorydefinitions.RequiredToolFailureKindMissing,
			Err:         errors.New(`required tool "Missing helper" command "missing-tool" was not found on PATH`),
		},
	}

	result := factoryvalidation.ValidateDeclarativeRequiredTools(cfg, checker)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking required-tool targets")
	}
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeRequiredToolMissing {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want typed missing required-tool target", result.Targets)
	}
}

func TestValidateOrchestratorTargets_ExportedHelperReturnsUnsupportedKindTarget(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		Name: "orchestrator-export",
		Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
			Kind: "LEGACY",
		},
	}

	result := factoryvalidation.ValidateOrchestratorTargets(cfg)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking orchestrator targets")
	}
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeOrchestratorUnsupportedKind {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want unsupported orchestrator kind target", result.Targets)
	}
}

package validation_test

import (
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
)

func workPropagationValidationBaseConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:   "execute-story",
			Inputs: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs: []interfaces.IOConfig{
				{WorkTypeName: "story", StateName: "complete"},
			},
			OnFailure: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
		}},
	}
}

func TestValidate_WorkPropagationSupportedModes(t *testing.T) {
	t.Parallel()

	for _, mode := range []interfaces.WorkPropagationMode{
		interfaces.WorkPropagationModeOutputAsPayload,
		interfaces.WorkPropagationModePreserveInput,
	} {
		t.Run(string(mode), func(t *testing.T) {
			cfg := workPropagationValidationBaseConfig()
			cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{Mode: mode}

			result := factoryvalidation.Validate(cfg)
			for _, target := range result.Targets {
				if target.Code == factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode {
					t.Fatalf("targets = %#v, want supported mode %q to stay valid", result.Targets, mode)
				}
			}
		})
	}
}

func TestValidate_WorkPropagationOmitted(t *testing.T) {
	t.Parallel()

	result := factoryvalidation.Validate(workPropagationValidationBaseConfig())
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode {
			t.Fatalf("targets = %#v, want omitted workPropagation to stay valid", result.Targets)
		}
	}
}

func TestValidate_WorkPropagationUnsupportedMode(t *testing.T) {
	t.Parallel()

	cfg := workPropagationValidationBaseConfig()
	cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{
		Mode: interfaces.WorkPropagationMode("MERGE_PAYLOAD"),
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode)

	var matched bool
	for _, target := range result.Targets {
		if target.Code != factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode {
			continue
		}
		matched = true
		if target.Subject.Type != factoryvalidation.SubjectTypeWorkstation || target.Subject.ID != "execute-story" {
			t.Fatalf("target subject = %#v, want workstation execute-story", target.Subject)
		}
		if target.Path != "factory.workstations[0](execute-story).workPropagation.mode" {
			t.Fatalf("target path = %q, want workstation workPropagation path", target.Path)
		}
		if !strings.Contains(target.Message, `unsupported workPropagation.mode "MERGE_PAYLOAD"`) {
			t.Fatalf("target message = %q, want unsupported mode detail", target.Message)
		}
	}
	if !matched {
		t.Fatal("expected unsupported work propagation target")
	}
}

func TestValidate_WorkPropagationEmptyMode(t *testing.T) {
	t.Parallel()

	cfg := workPropagationValidationBaseConfig()
	cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode)
}

func TestValidate_WorkPropagationUnsupportedModeUsesGeneratedEnumValues(t *testing.T) {
	t.Parallel()

	cfg := workPropagationValidationBaseConfig()
	cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{
		Mode: interfaces.WorkPropagationMode(factoryapi.WorkPropagationMode("PRESERVE_OUTPUT")),
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode)
}

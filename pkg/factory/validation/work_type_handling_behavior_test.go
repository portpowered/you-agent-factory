package validation_test

import (
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
)

func TestWorkTypeHandlingBehaviorTargets_RejectsMultipleDefaultWorkTypes(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "story", HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}},
			{Name: "task", HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}},
		},
	}

	targets := factoryvalidation.WorkTypeHandlingBehaviorTargets(cfg, factoryvalidation.WorkTypeHandlingBehaviorOptions{})
	validationassert.HasDomainTargetCode(t, targets, factoryvalidation.CodeWorkTypeHandlingBehaviorUniqueDefault)
}

func TestWorkTypeHandlingBehaviorTargets_AllowsSingleDefaultWorkType(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "story", HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}},
			{Name: "task"},
		},
	}

	targets := factoryvalidation.WorkTypeHandlingBehaviorTargets(cfg, factoryvalidation.WorkTypeHandlingBehaviorOptions{})
	if len(targets) != 0 {
		t.Fatalf("targets = %#v, want no handlingBehavior findings", targets)
	}
}

func TestValidate_RejectsDuplicateDefaultWorkTypesOnSavePath(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "story", HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}},
			{Name: "task", HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}},
		},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkTypeHandlingBehaviorUniqueDefault)
}

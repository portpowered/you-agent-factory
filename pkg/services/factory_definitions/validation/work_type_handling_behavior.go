package validation

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// WorkTypeHandlingBehaviorOptions configures optional handling-behavior validation.
type WorkTypeHandlingBehaviorOptions struct {
	RequireDefault bool
}

// WorkTypeHandlingBehaviorTargets validates handlingBehavior markers on work types.
func WorkTypeHandlingBehaviorTargets(cfg *factorydefinitions.FactoryConfig, opts WorkTypeHandlingBehaviorOptions) []Target {
	if cfg == nil {
		return nil
	}

	var targets []Target
	defaultHandlingCount := 0
	for workTypeIndex, workType := range cfg.WorkTypes {
		basePath := fmt.Sprintf("%s.workTypes[%d]", validationRoot, workTypeIndex)
		seenBehaviors := make(map[string]bool, len(workType.HandlingBehavior))
		workTypeDeclaresDefault := false
		for behaviorIndex, behavior := range workType.HandlingBehavior {
			behaviorPath := fmt.Sprintf("%s.handlingBehavior[%d]", basePath, behaviorIndex)
			canonical := factorydefinitions.StrictPublicWorkTypeHandlingBehavior(behavior)
			if canonical == "" {
				targets = append(targets, Target{
					Code:     CodeWorkTypeHandlingBehaviorValue,
					Severity: SeverityError,
					Message:  fmt.Sprintf("unsupported handlingBehavior value %q", behavior),
					Subject: Subject{
						Type:     SubjectTypeWorkType,
						ID:       factorydefinitions.CanonicalFactoryGraphWorkTypeID(workType),
						Location: SubjectLocationDefinition,
					},
					Path: behaviorPath,
				})
				continue
			}
			if seenBehaviors[canonical] {
				targets = append(targets, Target{
					Code:     CodeWorkTypeHandlingBehaviorDuplicate,
					Severity: SeverityError,
					Message:  fmt.Sprintf("duplicate handlingBehavior value %q on the same work type", canonical),
					Subject: Subject{
						Type:     SubjectTypeWorkType,
						ID:       factorydefinitions.CanonicalFactoryGraphWorkTypeID(workType),
						Location: SubjectLocationDefinition,
					},
					Path: behaviorPath,
				})
				continue
			}
			seenBehaviors[canonical] = true
			if canonical == factorydefinitions.WorkTypeHandlingBehaviorDefault {
				workTypeDeclaresDefault = true
			}
		}
		if workTypeDeclaresDefault {
			defaultHandlingCount++
		}
	}

	if defaultHandlingCount > 1 {
		targets = append(targets, Target{
			Code:     CodeWorkTypeHandlingBehaviorUniqueDefault,
			Severity: SeverityError,
			Message:  fmt.Sprintf("expected at most one work type with handlingBehavior DEFAULT, found %d", defaultHandlingCount),
			Subject: Subject{
				Type:     SubjectTypeFactory,
				ID:       "handlingBehavior",
				Location: SubjectLocationDefinition,
			},
			Path: fmt.Sprintf("%s.workTypes", validationRoot),
		})
	}
	if opts.RequireDefault && defaultHandlingCount == 0 {
		targets = append(targets, Target{
			Code:     CodeWorkTypeHandlingBehaviorRequiredDefault,
			Severity: SeverityError,
			Message:  "expected exactly one work type with handlingBehavior DEFAULT for simplified prompt runs",
			Subject: Subject{
				Type:     SubjectTypeFactory,
				ID:       "handlingBehavior",
				Location: SubjectLocationDefinition,
			},
			Path: fmt.Sprintf("%s.workTypes", validationRoot),
		})
	}
	return targets
}

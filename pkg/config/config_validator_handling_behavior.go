package config

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// WithRequireDefaultHandlingWorkType enables validation that exactly one work type
// declares handlingBehavior DEFAULT. Use this for simplified you run --factory flows.
func WithRequireDefaultHandlingWorkType() ConfigValidatorOption {
	return func(cv *ConfigValidator) {
		cv.requireDefaultHandlingWorkType = true
	}
}

func (cv *ConfigValidator) ruleWorkTypeHandlingBehavior(cfg *interfaces.FactoryConfig) []Finding {
	if cfg == nil {
		return nil
	}

	var findings []Finding
	defaultHandlingCount := 0
	for workTypeIndex, workType := range cfg.WorkTypes {
		basePath := fmt.Sprintf("workTypes[%d](%s)", workTypeIndex, workType.Name)
		seenBehaviors := make(map[string]bool, len(workType.HandlingBehavior))
		workTypeDeclaresDefault := false
		for behaviorIndex, behavior := range workType.HandlingBehavior {
			behaviorPath := fmt.Sprintf("%s.handlingBehavior[%d]", basePath, behaviorIndex)
			canonical := interfaces.StrictPublicWorkTypeHandlingBehavior(behavior)
			if canonical == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     behaviorPath,
					Message:  fmt.Sprintf("unsupported handlingBehavior value %q", behavior),
					Rule:     "work-type-handling-behavior-value",
				})
				continue
			}
			if seenBehaviors[canonical] {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     behaviorPath,
					Message:  fmt.Sprintf("duplicate handlingBehavior value %q on the same work type", canonical),
					Rule:     "work-type-handling-behavior-duplicate",
				})
				continue
			}
			seenBehaviors[canonical] = true
			if canonical == interfaces.WorkTypeHandlingBehaviorDefault {
				workTypeDeclaresDefault = true
			}
		}
		if workTypeDeclaresDefault {
			defaultHandlingCount++
		}
	}

	if defaultHandlingCount > 1 {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     "workTypes",
			Message:  fmt.Sprintf("expected at most one work type with handlingBehavior DEFAULT, found %d", defaultHandlingCount),
			Rule:     "work-type-handling-behavior-unique-default",
		})
	}
	if cv.requireDefaultHandlingWorkType && defaultHandlingCount == 0 {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     "workTypes",
			Message:  "expected exactly one work type with handlingBehavior DEFAULT for simplified prompt runs",
			Rule:     "work-type-handling-behavior-required-default",
		})
	}
	return findings
}

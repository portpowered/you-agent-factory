package config

import (
	"github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// CanonicalStructuralFindings returns structural validation findings for a factory
// definition through the canonical pkg/factory/validation entrypoint.
func CanonicalStructuralFindings(cfg *interfaces.FactoryConfig) []Finding {
	if cfg == nil {
		return nil
	}
	return canonicalTargetsToFindings(validation.Validate(cfg).Targets)
}

func canonicalTargetsToFindings(targets []validation.Target) []Finding {
	if len(targets) == 0 {
		return nil
	}
	findings := make([]Finding, 0, len(targets))
	for _, target := range targets {
		path := target.Path
		if path == "" {
			path = target.Subject.ID
		}
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     path,
			Message:  target.Message,
			Rule:     target.Code,
		})
	}
	return findings
}

func ruleCanonicalStructuralValidation(cfg *interfaces.FactoryConfig) []Finding {
	return CanonicalStructuralFindings(cfg)
}

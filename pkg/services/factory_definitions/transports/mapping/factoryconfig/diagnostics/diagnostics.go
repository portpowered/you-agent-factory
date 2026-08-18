// Package diagnostics maps canonical Factory Definition validation findings
// into the transport-facing configuration diagnostic representation.
package diagnostics

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Severity classifies the importance of a transport diagnostic.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityHint    Severity = "hint"
)

// Finding is the transport representation of one Factory configuration
// validation issue.
type Finding struct {
	Severity Severity
	Path     string
	Message  string
	Rule     string
}

// ValidationResult is the transport representation of validation findings.
type ValidationResult struct {
	Findings []Finding
}

func (r ValidationResult) HasErrors() bool {
	for _, finding := range r.Findings {
		if finding.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (r ValidationResult) Errors() []Finding {
	var errors []Finding
	for _, finding := range r.Findings {
		if finding.Severity == SeverityError {
			errors = append(errors, finding)
		}
	}
	return errors
}

func (r ValidationResult) Error() string {
	return factorydefinitions.TopologyValidationResult{
		Findings: canonicalTopologyFindings(r.Findings),
	}.Error()
}

// TopologyFindings converts canonical topology findings to transport
// diagnostics without applying validation policy.
func TopologyFindings(findings []factorydefinitions.TopologyFinding) []Finding {
	if len(findings) == 0 {
		return nil
	}
	mapped := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		mapped = append(mapped, Finding{
			Severity: Severity(finding.Severity),
			Path:     finding.Path,
			Message:  finding.Message,
			Rule:     finding.Rule,
		})
	}
	return mapped
}

// FactoryDefinitionFindings converts canonical validation targets to transport
// diagnostics.
func FactoryDefinitionFindings(targets []factorydefinitions.ValidationTarget) []Finding {
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
			Severity: Severity(target.Severity),
			Path:     path,
			Message:  target.Message,
			Rule:     target.Code,
		})
	}
	return findings
}

// BlockingFactoryLoadFindings converts a structured blocking-load error.
func BlockingFactoryLoadFindings(err error) []Finding {
	loadErr, ok := factorydefinitions.AsBlockingFactoryLoadError(err)
	if !ok {
		return nil
	}
	return FactoryDefinitionFindings(loadErr.Targets)
}

func canonicalTopologyFindings(findings []Finding) []factorydefinitions.TopologyFinding {
	if len(findings) == 0 {
		return nil
	}
	canonical := make([]factorydefinitions.TopologyFinding, 0, len(findings))
	for _, finding := range findings {
		canonical = append(canonical, factorydefinitions.TopologyFinding{
			Severity: factorydefinitions.ValidationSeverity(finding.Severity),
			Path:     finding.Path,
			Message:  finding.Message,
			Rule:     finding.Rule,
		})
	}
	return canonical
}

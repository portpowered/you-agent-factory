package impl

import (
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// ValidateGraphTopology runs graph reference validation for Petri-scoped factories
// and returns Definition-owned targets for dangling worker, resource, and route
// references without exposing Petri implementation types.
func ValidateGraphTopology(cfg *factorydefinitions.FactoryConfig) Result {
	if cfg == nil || !IsPetriOrchestratorValidationScope(cfg) {
		return Result{}
	}
	var targets []Target
	targets = append(targets, danglingReferenceTargets(cfg)...)
	targets = append(targets, invalidPlaceReferenceTargets(cfg)...)
	return Result{Targets: targets}
}

func validateLogicalRoundTrip(
	path string,
	guard factorydefinitions.GuardConfig,
	validWorkstations map[string]bool,
) []Finding {
	config := guard.LogicalRoundTrip
	if config == nil {
		return nil
	}

	if len(config.Workstations) != 2 {
		return []Finding{{
			Severity: SeverityError,
			Path:     path + ".logicalRoundTrip.workstations",
			Message:  "logical round-trip requires exactly two workstation names",
			Rule:     "guard-visit-count-round-trip-pair",
		}}
	}

	first := strings.TrimSpace(config.Workstations[0])
	second := strings.TrimSpace(config.Workstations[1])
	findings := validateLogicalRoundTripPair(path, first, second, validWorkstations)
	if guard.Workstation != "" && first != guard.Workstation && second != guard.Workstation {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     path + ".workstation",
			Message:  "logical round-trip pair must include the guard's watched workstation",
			Rule:     "guard-visit-count-round-trip-workstation",
		})
	}
	return append(findings, validateLogicalRoundTripBackstop(path, guard.MaxVisits, config.MaxRawVisits)...)
}

func validateLogicalRoundTripPair(
	path, first, second string,
	validWorkstations map[string]bool,
) []Finding {
	pairPath := path + ".logicalRoundTrip.workstations"
	var findings []Finding
	if first == "" || second == "" {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     pairPath,
			Message:  "logical round-trip workstation names must be non-empty",
			Rule:     "guard-visit-count-round-trip-pair",
		})
	}
	if first != "" && second != "" && first == second {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     pairPath,
			Message:  fmt.Sprintf("logical round-trip pair cannot name workstation %q twice", first),
			Rule:     "guard-visit-count-round-trip-pair",
		})
	}
	for index, workstation := range []string{first, second} {
		if workstation != "" && !validWorkstations[workstation] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     fmt.Sprintf("%s[%d]", pairPath, index),
				Message:  fmt.Sprintf("references non-existent workstation %q", workstation),
				Rule:     "guard-visit-count-round-trip-workstation",
			})
		}
	}
	return findings
}

func validateLogicalRoundTripBackstop(path string, maxVisits, maxRawVisits int) []Finding {
	var findings []Finding
	if maxVisits > 0 && maxRawVisits > 0 && maxRawVisits <= maxVisits {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     path + ".logicalRoundTrip.maxRawVisits",
			Message:  fmt.Sprintf("absolute raw-visit ceiling %d must be greater than logical maxVisits %d", maxRawVisits, maxVisits),
			Rule:     "guard-visit-count-round-trip-backstop",
		})
		return findings
	}
	if maxRawVisits <= 0 {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     path + ".logicalRoundTrip.maxRawVisits",
			Message:  "logical round-trip requires a positive absolute raw-visit ceiling",
			Rule:     "guard-visit-count-round-trip-backstop",
		})
	}
	return findings
}

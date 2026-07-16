package apisurface

import (
	"fmt"
	"io"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactoryRuntimeTaxonomyEntry summarizes one worker or workstation taxonomy value
// for CLI and API inspection output.
type FactoryRuntimeTaxonomyEntry struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Worker string `json:"worker,omitempty"`
}

// FactoryRuntimeTaxonomySummary projects authored worker and workstation taxonomy
// values without rewriting legacy aliases to newer names.
func FactoryRuntimeTaxonomySummary(factory factoryapi.Factory) []FactoryRuntimeTaxonomyEntry {
	var entries []FactoryRuntimeTaxonomyEntry
	if factory.Workers != nil {
		for _, worker := range *factory.Workers {
			entries = append(entries, FactoryRuntimeTaxonomyEntry{
				Kind: "worker",
				Name: strings.TrimSpace(worker.Name),
				Type: displayWorkerRuntimeType(worker.Type),
			})
		}
	}
	if factory.Workstations != nil {
		for _, workstation := range *factory.Workstations {
			entries = append(entries, FactoryRuntimeTaxonomyEntry{
				Kind:   "workstation",
				Name:   strings.TrimSpace(workstation.Name),
				Type:   displayWorkstationRuntimeType(workstation),
				Worker: strings.TrimSpace(workstation.Worker),
			})
		}
	}
	return entries
}

// RenderFactoryValidationHuman writes validate-only factory output with runtime
// taxonomy inspection lines and blocking validation targets.
func RenderFactoryValidationHuman(
	factory factoryapi.Factory,
	result factoryapi.FactoryValidationResult,
	output io.Writer,
) error {
	if len(result.Targets) == 0 {
		if _, err := fmt.Fprintln(output, "Factory validation passed."); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(output, "Factory validation failed."); err != nil {
			return err
		}
	}

	if entries := FactoryRuntimeTaxonomySummary(factory); len(entries) > 0 {
		if _, err := fmt.Fprintln(output, "Runtime taxonomy:"); err != nil {
			return err
		}
		for _, entry := range entries {
			switch entry.Kind {
			case "workstation":
				if _, err := fmt.Fprintf(
					output,
					"  workstation %s: %s (worker=%s)\n",
					entry.Name,
					entry.Type,
					entry.Worker,
				); err != nil {
					return err
				}
			default:
				if _, err := fmt.Fprintf(output, "  worker %s: %s\n", entry.Name, entry.Type); err != nil {
					return err
				}
			}
		}
	}

	if len(result.Targets) > 0 {
		if _, err := fmt.Fprintln(output, "Blocking targets:"); err != nil {
			return err
		}
		for _, target := range result.Targets {
			if _, err := fmt.Fprintf(output, "  %s\n", FormatFactoryValidationTarget(target)); err != nil {
				return err
			}
		}
		return fmt.Errorf("factory validation found blocking issues")
	}
	return nil
}

// FactoryValidationResultToAPI maps a canonical Factory validation result at
// the public transport boundary.
func FactoryValidationResultToAPI(result factoryvalidation.Result) factoryapi.FactoryValidationResult {
	return factoryapi.FactoryValidationResult{Targets: FactoryValidationTargetsToAPI(result.Targets)}
}

// FactoryValidationTargetsToAPI maps canonical Factory validation targets at
// the public transport boundary.
func FactoryValidationTargetsToAPI(targets []factoryvalidation.Target) []factoryapi.FactoryValidationTarget {
	if len(targets) == 0 {
		return []factoryapi.FactoryValidationTarget{}
	}
	mapped := make([]factoryapi.FactoryValidationTarget, 0, len(targets))
	for _, target := range targets {
		mapped = append(mapped, FactoryValidationTargetToAPI(target))
	}
	return mapped
}

// FactoryValidationTargetToAPI maps one canonical Factory validation target at
// the public transport boundary.
func FactoryValidationTargetToAPI(target factoryvalidation.Target) factoryapi.FactoryValidationTarget {
	return factoryapi.FactoryValidationTarget{
		Code:     target.Code,
		Severity: factoryValidationSeverityToAPI(target.Severity),
		Message:  target.Message,
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryValidationSubjectTypeToAPI(target.Subject.Type),
			Id:       target.Subject.ID,
			Location: factoryValidationSubjectLocationToAPI(target.Subject.Location),
		},
	}
}

func factoryValidationSeverityToAPI(severity factoryvalidation.Severity) factoryapi.FactoryValidationSeverity {
	switch severity {
	case factoryvalidation.SeverityWarning:
		return factoryapi.FactoryValidationSeverityWarning
	case factoryvalidation.SeverityHint:
		return factoryapi.FactoryValidationSeverityHint
	default:
		return factoryapi.FactoryValidationSeverityError
	}
}

func factoryValidationSubjectTypeToAPI(subjectType factoryvalidation.SubjectType) factoryapi.FactoryValidationSubjectType {
	switch subjectType {
	case factoryvalidation.SubjectTypeWorkstation:
		return factoryapi.FactoryValidationSubjectTypeWorkstation
	case factoryvalidation.SubjectTypeWorkType:
		return factoryapi.FactoryValidationSubjectTypeWorkType
	case factoryvalidation.SubjectTypeWorkState:
		return factoryapi.FactoryValidationSubjectTypeWorkState
	case factoryvalidation.SubjectTypeWorker:
		return factoryapi.FactoryValidationSubjectTypeWorker
	case factoryvalidation.SubjectTypeResource:
		return factoryapi.FactoryValidationSubjectTypeResource
	case factoryvalidation.SubjectTypeRoute:
		return factoryapi.FactoryValidationSubjectTypeRoute
	default:
		return factoryapi.FactoryValidationSubjectTypeFactory
	}
}

func factoryValidationSubjectLocationToAPI(location factoryvalidation.SubjectLocation) factoryapi.FactoryValidationSubjectLocation {
	switch location {
	case factoryvalidation.SubjectLocationOnRejection:
		return factoryapi.FactoryValidationSubjectLocationOnRejection
	case factoryvalidation.SubjectLocationOnFailure:
		return factoryapi.FactoryValidationSubjectLocationOnFailure
	case factoryvalidation.SubjectLocationOutputs:
		return factoryapi.FactoryValidationSubjectLocationOutputs
	case factoryvalidation.SubjectLocationInputs:
		return factoryapi.FactoryValidationSubjectLocationInputs
	case factoryvalidation.SubjectLocationStates:
		return factoryapi.FactoryValidationSubjectLocationStates
	case factoryvalidation.SubjectLocationTerminal:
		return factoryapi.FactoryValidationSubjectLocationTerminal
	case factoryvalidation.SubjectLocationReference:
		return factoryapi.FactoryValidationSubjectLocationReference
	default:
		return factoryapi.FactoryValidationSubjectLocationDefinition
	}
}

// FactoryTopologyValidationErrorInput maps a canonical validation result into
// the transport-owned topology error input.
func FactoryTopologyValidationErrorInput(
	result factoryvalidation.Result,
	message string,
) (string, []factoryapi.FactoryValidationTarget) {
	if message == "" {
		message = factoryvalidation.DefaultTopologyValidationMessage
	}
	return message, FactoryValidationTargetsToAPI(result.Targets)
}

// FormatFactoryValidationTarget renders one API validation target for human CLI output.
func FormatFactoryValidationTarget(target factoryapi.FactoryValidationTarget) string {
	subject := strings.TrimSpace(string(target.Subject.Type))
	if subject == "" {
		subject = "FACTORY"
	}
	subjectID := strings.TrimSpace(target.Subject.Id)
	location := strings.TrimSpace(string(target.Subject.Location))
	parts := []string{string(target.Severity), target.Code}
	if subjectID != "" {
		parts = append(parts, fmt.Sprintf("%s(%s)", subject, subjectID))
	} else {
		parts = append(parts, subject)
	}
	if location != "" {
		parts = append(parts, location)
	}
	return fmt.Sprintf("%s: %s", strings.Join(parts, " "), strings.TrimSpace(target.Message))
}

func displayWorkerRuntimeType(workerType *factoryapi.WorkerType) string {
	if workerType == nil {
		return "unspecified worker type"
	}
	trimmed := strings.TrimSpace(string(*workerType))
	if trimmed == "" {
		return "unspecified worker type"
	}
	return trimmed
}

func displayWorkstationRuntimeType(workstation factoryapi.Workstation) string {
	if workstation.Type != nil {
		trimmed := strings.TrimSpace(string(*workstation.Type))
		if trimmed != "" {
			return trimmed
		}
	}
	if workstation.Behavior != nil {
		normalized := interfaces.CanonicalPublicWorkstationKind(
			interfaces.WorkstationKind(strings.TrimSpace(string(*workstation.Behavior))),
		)
		if normalized == string(factoryapi.WorkstationKindPoller) {
			return "legacy poller kind"
		}
	}
	return "legacy agent-run default"
}

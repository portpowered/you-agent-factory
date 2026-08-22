package apisurface

import (
	"fmt"
	"io"
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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

// FactoryConfigIgnoredFieldWarningCode identifies a forward-compatible field
// that was ignored while decoding a customer-authored Factory Definition.
const FactoryConfigIgnoredFieldWarningCode = interfaces.FactoryConfigIgnoredFieldWarningCode

// FactoryConfigDecodeWarning is the safe public warning shape used by Factory
// validation output. It intentionally contains a path and no decoded value.
type FactoryConfigDecodeWarning struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

// FactoryConfigDecodeWarnings maps ignored paths into deterministic warning
// records for transport output.
func FactoryConfigDecodeWarnings(paths []string) []FactoryConfigDecodeWarning {
	unique := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			unique[path] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(unique))
	for path := range unique {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	warnings := make([]FactoryConfigDecodeWarning, 0, len(ordered))
	for _, path := range ordered {
		warnings = append(warnings, FactoryConfigDecodeWarning{
			Code: FactoryConfigIgnoredFieldWarningCode,
			Path: path,
		})
	}
	return warnings
}

// RenderFactoryConfigDecodeWarnings writes the human-readable compatibility
// warnings produced by Factory config validation.
func RenderFactoryConfigDecodeWarnings(paths []string, output io.Writer) error {
	warnings := FactoryConfigDecodeWarnings(paths)
	if len(warnings) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(output, "Warnings:"); err != nil {
		return err
	}
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(
			output,
			"  warning: ignored unknown Factory field at %s (%s)\n",
			warning.Path,
			warning.Code,
		); err != nil {
			return err
		}
	}
	return nil
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
				Worker: strings.TrimSpace(optionalString(workstation.Worker)),
			})
		}
	}
	return entries
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
func FactoryValidationResultToAPI(result interfaces.ValidationResult) factoryapi.FactoryValidationResult {
	return factoryapi.FactoryValidationResult{Targets: FactoryValidationTargetsToAPI(result.Targets)}
}

// FactoryValidationTargetsToAPI maps canonical Factory validation targets at
// the public transport boundary.
func FactoryValidationTargetsToAPI(targets []interfaces.ValidationTarget) []factoryapi.FactoryValidationTarget {
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
func FactoryValidationTargetToAPI(target interfaces.ValidationTarget) factoryapi.FactoryValidationTarget {
	return factoryapi.FactoryValidationTarget{
		Code:     target.Code,
		Severity: factoryValidationSeverityToAPI(target.Severity),
		Message:  target.Message,
		Path:     factoryValidationPathToAPI(target.Path),
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryValidationSubjectTypeToAPI(target.Subject.Type),
			Id:       target.Subject.ID,
			Location: factoryValidationSubjectLocationToAPI(target.Subject.Location),
		},
	}
}

func factoryValidationPathToAPI(path string) *string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return &path
}

func factoryValidationSeverityToAPI(severity interfaces.ValidationSeverity) factoryapi.FactoryValidationSeverity {
	switch severity {
	case interfaces.ValidationSeverityWarning:
		return factoryapi.FactoryValidationSeverityWarning
	case interfaces.ValidationSeverityHint:
		return factoryapi.FactoryValidationSeverityHint
	default:
		return factoryapi.FactoryValidationSeverityError
	}
}

func factoryValidationSubjectTypeToAPI(subjectType interfaces.ValidationSubjectType) factoryapi.FactoryValidationSubjectType {
	switch subjectType {
	case interfaces.ValidationSubjectTypeWorkstation:
		return factoryapi.FactoryValidationSubjectTypeWorkstation
	case interfaces.ValidationSubjectTypeWorkType:
		return factoryapi.FactoryValidationSubjectTypeWorkType
	case interfaces.ValidationSubjectTypeWorkState:
		return factoryapi.FactoryValidationSubjectTypeWorkState
	case interfaces.ValidationSubjectTypeWorker:
		return factoryapi.FactoryValidationSubjectTypeWorker
	case interfaces.ValidationSubjectTypeResource:
		return factoryapi.FactoryValidationSubjectTypeResource
	case interfaces.ValidationSubjectTypeRoute:
		return factoryapi.FactoryValidationSubjectTypeRoute
	default:
		return factoryapi.FactoryValidationSubjectTypeFactory
	}
}

func factoryValidationSubjectLocationToAPI(location interfaces.ValidationSubjectLocation) factoryapi.FactoryValidationSubjectLocation {
	switch location {
	case interfaces.ValidationSubjectLocationOnRejection:
		return factoryapi.FactoryValidationSubjectLocationOnRejection
	case interfaces.ValidationSubjectLocationOnFailure:
		return factoryapi.FactoryValidationSubjectLocationOnFailure
	case interfaces.ValidationSubjectLocationOutputs:
		return factoryapi.FactoryValidationSubjectLocationOutputs
	case interfaces.ValidationSubjectLocationInputs:
		return factoryapi.FactoryValidationSubjectLocationInputs
	case interfaces.ValidationSubjectLocationStates:
		return factoryapi.FactoryValidationSubjectLocationStates
	case interfaces.ValidationSubjectLocationTerminal:
		return factoryapi.FactoryValidationSubjectLocationTerminal
	case interfaces.ValidationSubjectLocationReference:
		return factoryapi.FactoryValidationSubjectLocationReference
	default:
		return factoryapi.FactoryValidationSubjectLocationDefinition
	}
}

// FactoryTopologyValidationErrorInput maps a canonical validation result into
// the transport-owned topology error input.
func FactoryTopologyValidationErrorInput(
	result interfaces.ValidationResult,
	message string,
) (string, []factoryapi.FactoryValidationTarget) {
	if message == "" {
		message = interfaces.DefaultTopologyValidationMessage
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

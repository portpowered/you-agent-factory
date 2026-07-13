package apisurface

import (
	"fmt"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
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

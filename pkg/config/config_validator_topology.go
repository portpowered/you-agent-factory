package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/timework"
)

func ruleInputTypes(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	seen := make(map[string]bool)
	for i, it := range cfg.InputTypes {
		path := fmt.Sprintf("input_types[%d]", i)
		if it.Name == "" {
			findings = append(findings, Finding{
				Severity: SeverityError, Path: path,
				Message: "missing required 'name' field", Rule: "input-type-name",
			})
			continue
		}
		pathNamed := fmt.Sprintf("input_types[%d](%s)", i, it.Name)
		if it.Name == "default" {
			findings = append(findings, Finding{
				Severity: SeverityError, Path: path,
				Message: "'default' is an implicit input type and must not be declared", Rule: "input-type-reserved",
			})
		}
		if seen[it.Name] {
			findings = append(findings, Finding{
				Severity: SeverityError, Path: path,
				Message: fmt.Sprintf("duplicate input type name %q", it.Name), Rule: "input-type-duplicate",
			})
		}
		seen[it.Name] = true

		switch it.Type {
		case interfaces.InputKindDefault:
			// valid
		case "":
			findings = append(findings, Finding{
				Severity: SeverityError, Path: pathNamed,
				Message: "missing required 'type' field", Rule: "input-type-type",
			})
		default:
			findings = append(findings, Finding{
				Severity: SeverityError, Path: pathNamed,
				Message: fmt.Sprintf("unknown input type %q (supported: %q)", it.Type, interfaces.InputKindDefault),
				Rule:    "input-type-type",
			})
		}
	}
	return findings
}

// --- Rule: place reference validation ---

func rulePlaceReferences(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	validPlaces := buildValidPlaces(cfg)

	for wi, ws := range cfg.Workstations {
		for ii, input := range ws.Inputs {
			if !validPlaces[mapToID(input)] {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     fmt.Sprintf("workstations[%d](%s).inputs[%d]", wi, ws.Name, ii),
					Message:  fmt.Sprintf("references non-existent state %q of work type %q", input.StateName, input.WorkTypeName),
					Rule:     "workstation-input-ref",
				})
			}
		}
		for oi, output := range ws.Outputs {
			if !validPlaces[mapToID(output)] {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     fmt.Sprintf("workstations[%d](%s).outputs[%d]", wi, ws.Name, oi),
					Message:  fmt.Sprintf("references non-existent state %q of work type %q", output.StateName, output.WorkTypeName),
					Rule:     "workstation-output-ref",
				})
			}
		}
		for oi, route := range ws.OnContinue {
			if validPlaces[mapToID(route)] {
				continue
			}
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     fmt.Sprintf("workstations[%d](%s).on_continue[%d]", wi, ws.Name, oi),
				Message:  fmt.Sprintf("references non-existent state %q of work type %q", route.StateName, route.WorkTypeName),
				Rule:     "workstation-on-continue-ref",
			})
		}
		for oi, route := range ws.OnRejection {
			if validPlaces[mapToID(route)] {
				continue
			}
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     fmt.Sprintf("workstations[%d](%s).on_rejection[%d]", wi, ws.Name, oi),
				Message:  fmt.Sprintf("references non-existent state %q of work type %q", route.StateName, route.WorkTypeName),
				Rule:     "workstation-on-rejection-ref",
			})
		}
		for oi, route := range ws.OnFailure {
			if validPlaces[mapToID(route)] {
				continue
			}
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     fmt.Sprintf("workstations[%d](%s).on_failure[%d]", wi, ws.Name, oi),
				Message:  fmt.Sprintf("references non-existent state %q of work type %q", route.StateName, route.WorkTypeName),
				Rule:     "workstation-on-failure-ref",
			})
		}
	}
	return findings
}

// --- Rule: guard validation ---

func ruleFactoryGuards(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding

	for gi, g := range cfg.Guards {
		basePath := fmt.Sprintf("guards[%d](%s)", gi, g.Type)
		switch g.Type {
		case interfaces.GuardTypeInferenceThrottle:
			modelProvider := strings.TrimSpace(g.ModelProvider)
			if modelProvider == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".modelProvider",
					Message:  "factory guard requires non-empty 'modelProvider'",
					Rule:     "factory-guard-inference-throttle-model-provider",
				})
			}

			refreshWindow := strings.TrimSpace(g.RefreshWindow)
			switch {
			case refreshWindow == "":
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".refreshWindow",
					Message:  "factory guard requires non-empty 'refreshWindow'",
					Rule:     "factory-guard-inference-throttle-refresh-window",
				})
			default:
				duration, err := time.ParseDuration(refreshWindow)
				if err != nil || duration <= 0 {
					findings = append(findings, Finding{
						Severity: SeverityError,
						Path:     basePath + ".refreshWindow",
						Message:  fmt.Sprintf("refreshWindow must be a positive duration, got %q", g.RefreshWindow),
						Rule:     "factory-guard-inference-throttle-refresh-window",
					})
				}
			}
		default:
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath,
				Message:  fmt.Sprintf("unsupported factory guard type %q (factory guards support: inference_throttle_guard)", g.Type),
				Rule:     "factory-guard-unknown-type",
			})
		}
	}

	return findings
}

func ruleGuards(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	validWorkstations := buildValidWorkstations(cfg)

	for wi, ws := range cfg.Workstations {
		for gi, g := range ws.Guards {
			path := fmt.Sprintf("workstations[%d](%s).guards[%d]", wi, ws.Name, gi)
			switch g.Type {
			case interfaces.GuardTypeVisitCount:
				if g.Workstation == "" {
					findings = append(findings, Finding{
						Severity: SeverityError, Path: path,
						Message: fmt.Sprintf("guard of type %q requires 'workstation' parameter", g.Type),
						Rule:    "guard-visit-count-workstation",
					})
				} else if !validWorkstations[g.Workstation] {
					findings = append(findings, Finding{
						Severity: SeverityError, Path: path,
						Message: fmt.Sprintf("references non-existent workstation %q", g.Workstation),
						Rule:    "guard-visit-count-workstation",
					})
				}
				if g.MaxVisits <= 0 {
					findings = append(findings, Finding{
						Severity: SeverityError, Path: path,
						Message: fmt.Sprintf("guard of type %q requires positive 'max_visits'", g.Type),
						Rule:    "guard-visit-count-max-visits",
					})
				}
			case interfaces.GuardTypeMatchesFields:
				if g.MatchConfig == nil || strings.TrimSpace(g.MatchConfig.InputKey) == "" {
					findings = append(findings, Finding{
						Severity: SeverityError, Path: path,
						Message: fmt.Sprintf("guard of type %q requires non-empty 'match_config.input_key'", g.Type),
						Rule:    "guard-matches-fields-input-key",
					})
				}
			default:
				findings = append(findings, Finding{
					Severity: SeverityError, Path: path,
					Message: fmt.Sprintf("unsupported workstation guard type %q (workstation guards support: visit_count, matches_fields; use per-input guards for child fan-in)", g.Type),
					Rule:    "guard-unknown-type",
				})
			}
		}
	}
	return findings
}

// --- Rule: workstation kind validation ---

func ruleWorkstationKind(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	validKinds := map[interfaces.WorkstationKind]bool{
		interfaces.WorkstationKindStandard: true,
		interfaces.WorkstationKindRepeater: true,
		interfaces.WorkstationKindCron:     true,
		interfaces.WorkstationKindPoller:   true,
	}
	for wi, ws := range cfg.Workstations {
		if ws.Kind == "" {
			continue
		}
		if !validKinds[ws.Kind] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     fmt.Sprintf("workstations[%d](%s).kind", wi, ws.Name),
				Message:  fmt.Sprintf("unknown kind %q (valid kinds: standard, repeater, cron)", ws.Kind),
				Rule:     "workstation-kind",
			})
		}
	}
	return findings
}

// --- Rule: poller workstation validation ---

func rulePollerWorkstations(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding

	workersByName := make(map[string]interfaces.WorkerConfig, len(cfg.Workers))
	for _, worker := range cfg.Workers {
		workersByName[worker.Name] = worker
	}

	for wi, ws := range cfg.Workstations {
		if ws.Kind != interfaces.WorkstationKindPoller {
			continue
		}

		basePath := fmt.Sprintf("workstations[%d](%s)", wi, ws.Name)
		if strings.TrimSpace(ws.WorkerTypeName) == "" {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".worker",
				Message:  "poller workstation requires a worker because pollers only run through a bound worker in v1",
				Rule:     "poller-worker",
			})
			continue
		}

		worker, ok := workersByName[ws.WorkerTypeName]
		if !ok {
			continue
		}
		switch strings.TrimSpace(worker.Type) {
		case interfaces.WorkerTypeScript, interfaces.WorkerTypeHosted:
			continue
		default:
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".worker",
				Message: fmt.Sprintf(
					"poller workstation %q cannot bind worker %q of type %q; v1 pollers support only SCRIPT_WORKER or HOSTED_WORKER",
					ws.Name,
					worker.Name,
					worker.Type,
				),
				Rule: "poller-worker-type",
			})
		}
	}

	return findings
}

// --- Rule: cron workstation validation ---

// portos:func-length-exception owner=agent-factory reason=cron-validation-rule-table review=2026-07-18 removal=split-cron-field-validators-before-adding-more-cron-options
func ruleCronWorkstations(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding

	for wi, ws := range cfg.Workstations {
		basePath := fmt.Sprintf("workstations[%d](%s)", wi, ws.Name)

		if ws.Kind != interfaces.WorkstationKindCron {
			if ws.Cron != nil {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".cron",
					Message:  "cron configuration is only valid when kind is \"cron\"",
					Rule:     "cron-type",
				})
			}
			continue
		}

		if ws.Cron == nil {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".cron",
				Message:  "cron workstation requires a 'cron' configuration object",
				Rule:     "cron-config",
			})
			continue
		}

		if ws.Cron.HasUnsupportedInterval() {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".cron.interval",
				Message:  "cron.interval is not supported; use cron.schedule",
				Rule:     "cron-interval",
			})
		}

		hasSchedule := strings.TrimSpace(ws.Cron.Schedule) != ""
		if !hasSchedule {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".cron.schedule",
				Message:  "cron workstation requires non-empty 'schedule'",
				Rule:     "cron-schedule",
			})
		} else if err := timework.ValidateCronSchedule(ws.Cron.Schedule); err != nil {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".cron.schedule",
				Message:  err.Error(),
				Rule:     "cron-schedule",
			})
		}
		if strings.TrimSpace(ws.Cron.Jitter) != "" {
			if _, err := timework.ParseCronJitter(ws.Cron); err != nil {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".cron.jitter",
					Message:  fmt.Sprintf("jitter must be a non-negative duration, got %q", ws.Cron.Jitter),
					Rule:     "cron-jitter",
				})
			}
		}
		if strings.TrimSpace(ws.Cron.ExpiryWindow) != "" {
			if _, err := timework.ParseCronExpiryWindow(ws.Cron, 1); err != nil {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".cron.expiry_window",
					Message:  fmt.Sprintf("expiry_window must be a positive duration, got %q", ws.Cron.ExpiryWindow),
					Rule:     "cron-expiry-window",
				})
			}
		}
		if len(ws.Outputs) == 0 {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".outputs",
				Message:  "cron workstation requires at least one configured output",
				Rule:     "cron-output",
			})
		}
		if strings.TrimSpace(ws.WorkerTypeName) == "" {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".worker",
				Message:  "cron workstation requires a worker because cron dispatches through the normal worker path",
				Rule:     "cron-worker",
			})
		}
	}

	return findings
}

// --- Rule: worker reference validation ---

func ruleWorkerReferences(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	validWorkers := make(map[string]bool)
	for _, w := range cfg.Workers {
		validWorkers[w.Name] = true
	}
	for wi, ws := range cfg.Workstations {
		if ws.WorkerTypeName != "" && !validWorkers[ws.WorkerTypeName] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     fmt.Sprintf("workstations[%d](%s).worker", wi, ws.Name),
				Message:  fmt.Sprintf("references non-existent worker %q", ws.WorkerTypeName),
				Rule:     "workstation-worker-ref",
			})
		}
	}
	return findings
}

func ruleHostedWorkers(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	for wi, worker := range cfg.Workers {
		basePath := fmt.Sprintf("workers[%d](%s)", wi, worker.Name)
		switch worker.Type {
		case interfaces.WorkerTypeHosted:
			if strings.TrimSpace(worker.Provider) == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".provider",
					Message:  "hosted worker requires non-empty 'provider'",
					Rule:     "hosted-worker-provider",
				})
			}
			if worker.Provider != interfaces.HostedWorkerProviderLinear {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".provider",
					Message:  fmt.Sprintf("unsupported hosted worker provider %q (supported: LINEAR)", worker.Provider),
					Rule:     "hosted-worker-provider",
				})
			}
			if worker.Auth == nil || strings.TrimSpace(worker.Auth.SecretRef) == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".auth.secretRef",
					Message:  "hosted worker requires auth.secretRef",
					Rule:     "hosted-worker-auth-secret-ref",
				})
			}
			if worker.Linear == nil {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".linear",
					Message:  "LINEAR hosted worker requires provider-specific linear configuration",
					Rule:     "hosted-worker-linear-config",
				})
				continue
			}
			if strings.TrimSpace(worker.Linear.Mapping.WorkType) == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".linear.mapping.workType",
					Message:  "LINEAR hosted worker requires linear.mapping.workType",
					Rule:     "hosted-worker-linear-mapping-work-type",
				})
			}
			if strings.TrimSpace(worker.Linear.Mapping.State) == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".linear.mapping.state",
					Message:  "LINEAR hosted worker requires linear.mapping.state",
					Rule:     "hosted-worker-linear-mapping-state",
				})
			}
			if worker.Linear.Claim != nil && strings.TrimSpace(worker.Linear.Claim.AssigneeField) == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".linear.claim.assigneeField",
					Message:  "LINEAR hosted worker claim config requires non-empty assigneeField when claim is present",
					Rule:     "hosted-worker-linear-claim-assignee-field",
				})
			}
		default:
			if strings.TrimSpace(worker.Provider) != "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".provider",
					Message:  fmt.Sprintf("worker type %q cannot declare hosted provider configuration", worker.Type),
					Rule:     "hosted-worker-provider-unsupported",
				})
			}
			if worker.Auth != nil {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".auth",
					Message:  fmt.Sprintf("worker type %q cannot declare hosted auth configuration", worker.Type),
					Rule:     "hosted-worker-auth-unsupported",
				})
			}
			if worker.Linear != nil {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".linear",
					Message:  fmt.Sprintf("worker type %q cannot declare hosted LINEAR configuration", worker.Type),
					Rule:     "hosted-worker-linear-unsupported",
				})
			}
		}
	}
	return findings
}

// --- Rule: per-input guard validation ---

func rulePerInputGuards(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	validWorkstations := buildValidWorkstations(cfg)

	for wi, ws := range cfg.Workstations {
		inputWorkTypes := perInputGuardWorkTypes(ws.Inputs)

		for ii, input := range ws.Inputs {
			if input.Guard == nil {
				continue
			}
			path := fmt.Sprintf("workstations[%d](%s).inputs[%d].guard", wi, ws.Name, ii)
			findings = append(findings, validatePerInputGuard(input, path, inputWorkTypes, validWorkstations)...)
		}
	}
	return findings
}

func perInputGuardWorkTypes(inputs []interfaces.IOConfig) map[string]bool {
	workTypes := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		workTypes[input.WorkTypeName] = true
	}
	return workTypes
}

func validatePerInputGuard(input interfaces.IOConfig, path string, inputWorkTypes, validWorkstations map[string]bool) []Finding {
	switch input.Guard.Type {
	case interfaces.GuardTypeAllChildrenComplete, interfaces.GuardTypeAnyChildFailed:
		return validateParentAwareInputGuard(input, path, inputWorkTypes, validWorkstations)
	case interfaces.GuardTypeSameName:
		return validateSameNameInputGuard(input, path, inputWorkTypes)
	case interfaces.GuardTypeSameTraceID:
		return validateSameTraceIDInputGuard(input, path, inputWorkTypes)
	default:
		return []Finding{{
			Severity: SeverityError,
			Path:     path,
			Message:  fmt.Sprintf("unsupported guard type %q (per-input guards support: all_children_complete, any_child_failed, same_name, same_trace_id)", input.Guard.Type),
			Rule:     "per-input-guard-type",
		}}
	}
}

func validateParentAwareInputGuard(input interfaces.IOConfig, path string, inputWorkTypes, validWorkstations map[string]bool) []Finding {
	findings := validatePeerInputReference(
		path,
		"parent_input",
		input.Guard.Type,
		input.Guard.ParentInput,
		input.WorkTypeName,
		inputWorkTypes,
		"per-input-guard-parent-input",
		"per-input-guard-self-ref",
	)
	if input.Guard.SpawnedBy != "" && !validWorkstations[input.Guard.SpawnedBy] {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     path,
			Message:  fmt.Sprintf("spawned_by references non-existent workstation %q", input.Guard.SpawnedBy),
			Rule:     "per-input-guard-spawned-by",
		})
	}
	return findings
}

func validateSameNameInputGuard(input interfaces.IOConfig, path string, inputWorkTypes map[string]bool) []Finding {
	return validatePeerInputReference(
		path,
		"match_input",
		input.Guard.Type,
		input.Guard.MatchInput,
		input.WorkTypeName,
		inputWorkTypes,
		"per-input-guard-match-input",
		"per-input-guard-self-ref",
	)
}

func validateSameTraceIDInputGuard(input interfaces.IOConfig, path string, inputWorkTypes map[string]bool) []Finding {
	return validatePeerInputReference(
		path,
		"match_input",
		input.Guard.Type,
		input.Guard.MatchInput,
		input.WorkTypeName,
		inputWorkTypes,
		"per-input-guard-same-trace-match-input",
		"per-input-guard-same-trace-self-ref",
	)
}

func validatePeerInputReference(path, fieldName string, guardType interfaces.GuardType, reference, workTypeName string, inputWorkTypes map[string]bool, fieldRule, selfRule string) []Finding {
	if reference == "" {
		return []Finding{{
			Severity: SeverityError,
			Path:     path,
			Message:  fmt.Sprintf("guard of type %q requires %q", guardType, fieldName),
			Rule:     fieldRule,
		}}
	}

	var findings []Finding
	if !inputWorkTypes[reference] {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     path,
			Message:  fmt.Sprintf("%s %q does not match any input work type", fieldName, reference),
			Rule:     fieldRule,
		})
	}
	if reference == workTypeName {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     path,
			Message:  fmt.Sprintf("%s %q cannot reference its own input", fieldName, reference),
			Rule:     selfRule,
		})
	}
	return findings
}

// --- Rule: resource usage validation ---

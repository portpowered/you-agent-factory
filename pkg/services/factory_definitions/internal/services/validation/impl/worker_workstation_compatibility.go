package impl

import (
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
)

// WorkerWorkstationBehaviorCompatibilityTargets returns canonical validation targets
// for incompatible worker/workstation runtime taxonomy pairings.
func WorkerWorkstationBehaviorCompatibilityTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg == nil || len(cfg.Workstations) == 0 {
		return nil
	}

	workersByName := make(map[string]workerconfig.Config, len(cfg.Workers))
	for _, worker := range cfg.Workers {
		workersByName[worker.Name] = worker
	}

	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		if workstation.Kind == factorydefinitions.WorkstationKindPoller &&
			strings.TrimSpace(workstation.Type) == "" {
			continue
		}
		if !factorydefinitions.RequiresWorkerWorkstationBehaviorCompatibility(
			workstation.Type,
			workstation.Kind,
			workstation.WorkerTypeName,
		) {
			continue
		}

		worker, ok := workersByName[workstation.WorkerTypeName]
		if !ok {
			continue
		}
		if factorydefinitions.CompatibleWorkerWorkstationBehavior(
			worker.Type, workstation.Type, workstation.Kind,
		) {
			continue
		}

		basePath := fmt.Sprintf("%s.workstations[%d](%s)", validationRoot, workstationIndex, workstation.Name)
		targets = append(targets, Target{
			Code:     CodeWorkerWorkstationBehaviorCompatibility,
			Severity: SeverityError,
			Message: factorydefinitions.WorkerWorkstationBehaviorMismatchMessage(
				workstation.Name,
				workstation.Type,
				workstation.Kind,
				worker.Name,
				worker.Type,
			),
			Subject: Subject{
				Type:     SubjectTypeWorkstation,
				ID:       workstation.Name,
				Location: SubjectLocationReference,
			},
			Path: basePath + ".worker",
		})
	}

	return targets
}

// humanApprovalWorkstationTargets owns the authoring contract for the one
// workerless workstation type. Keeping these checks at the definition boundary
// prevents a human gate from being silently interpreted as a runnable worker
// workstation by a mapper or transport.
func humanApprovalWorkstationTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg == nil {
		return nil
	}

	var targets []Target
	for index, workstation := range cfg.Workstations {
		if strings.TrimSpace(workstation.Type) != factorydefinitions.WorkstationTypeHumanApproval {
			continue
		}
		basePath := fmt.Sprintf("%s.workstations[%d](%s)", validationRoot, index, workstation.Name)

		if strings.TrimSpace(workstation.ID) == "" {
			targets = append(targets, humanApprovalTarget(workstation, basePath+".id", "HUMAN_APPROVAL workstation requires a stable id"))
		}
		if workstation.Description == nil {
			targets = append(targets, humanApprovalTarget(workstation, basePath+".description", "HUMAN_APPROVAL workstation requires a localizable description"))
		} else if err := factorydefinitions.ValidateNameValue(*workstation.Description); err != nil {
			targets = append(targets, humanApprovalTarget(workstation, basePath+".description."+nameValueErrorField(err), fmt.Sprintf("HUMAN_APPROVAL workstation description is invalid: %v", err)))
		}
		if len(workstation.Inputs) == 0 {
			targets = append(targets, humanApprovalTarget(workstation, basePath+".inputs", "HUMAN_APPROVAL workstation requires at least one input"))
		}
		if len(workstation.Outputs) == 0 {
			targets = append(targets, humanApprovalTarget(workstation, basePath+".outputs", "HUMAN_APPROVAL workstation requires at least one approval output"))
		}
		if len(workstation.OnRejection) == 0 {
			targets = append(targets, humanApprovalTarget(workstation, basePath+".onRejection", "HUMAN_APPROVAL workstation requires at least one rejection output"))
		}

		if workstation.Kind != "" && workstation.Kind != factorydefinitions.WorkstationKindStandard {
			targets = append(targets, humanApprovalTarget(workstation, basePath+".behavior", "HUMAN_APPROVAL workstations must use STANDARD behavior"))
		}
		if strings.TrimSpace(workstation.WorkerTypeName) != "" {
			targets = append(targets, humanApprovalTarget(workstation, basePath+".worker", "HUMAN_APPROVAL workstations must not declare a worker"))
		}
		appendHumanApprovalUnsupportedTargets(&targets, workstation, basePath)

		if factorydefinitions.IsJavaScriptOrchestratorFactory(cfg) {
			targets = append(targets, humanApprovalTarget(workstation, basePath+".type", "HUMAN_APPROVAL workstations are supported only by the graph/Petri orchestrator"))
		}
	}
	return targets
}

func appendHumanApprovalUnsupportedTargets(targets *[]Target, workstation factorydefinitions.FactoryWorkstationConfig, basePath string) {
	unsupported := []struct {
		path  string
		used  bool
		label string
	}{
		{path: ".runner", used: strings.TrimSpace(workstation.Runner) != "", label: "runner"},
		{path: ".operation", used: strings.TrimSpace(workstation.Operation) != "", label: "provider/model operation"},
		{path: ".operationBindings", used: len(workstation.OperationBindings) != 0, label: "provider/model operation bindings"},
		{path: ".promptFile", used: strings.TrimSpace(workstation.PromptFile) != "", label: "prompt"},
		{path: ".prompt_template", used: strings.TrimSpace(workstation.PromptTemplate) != "", label: "prompt"},
		{path: ".body", used: strings.TrimSpace(workstation.Body) != "", label: "body"},
		{path: ".outputSchema", used: strings.TrimSpace(workstation.OutputSchema) != "", label: "output schema"},
		{path: ".outputContract", used: strings.TrimSpace(workstation.OutputContract) != "", label: "output contract"},
		{path: ".outcomeFormat", used: strings.TrimSpace(workstation.OutcomeFormat) != "", label: "outcome format"},
		{path: ".timeout", used: strings.TrimSpace(workstation.Timeout) != "", label: "execution timeout"},
		{path: ".limits", used: workstation.Limits != (factorydefinitions.WorkstationLimits{}), label: "execution limits"},
		{path: ".workPropagation", used: workstation.WorkPropagation != nil, label: "work propagation"},
		{path: ".cron", used: workstation.Cron != nil, label: "cron trigger"},
		{path: ".expectedArtifacts", used: len(workstation.ExpectedArtifacts) != 0, label: "expected artifacts"},
		{path: ".resources", used: len(workstation.Resources) != 0, label: "resources"},
		{path: ".copyReferencedScripts", used: workstation.CopyReferencedScripts, label: "referenced scripts"},
		{path: ".guards", used: len(workstation.Guards) != 0, label: "guards"},
		{path: ".stopWords", used: len(workstation.StopWords) != 0, label: "stop words"},
		{path: ".runtime_stop_words", used: len(workstation.RuntimeStopWords) != 0, label: "runtime stop words"},
		{path: ".classification_routes", used: len(workstation.ClassificationRoutes) != 0, label: "classification routes"},
		{path: ".onContinue", used: len(workstation.OnContinue) != 0, label: "onContinue route"},
		{path: ".onFailure", used: len(workstation.OnFailure) != 0, label: "onFailure route"},
		{path: ".workingDirectory", used: strings.TrimSpace(workstation.WorkingDirectory) != "", label: "working directory"},
		{path: ".worktree", used: strings.TrimSpace(workstation.Worktree) != "", label: "worktree"},
		{path: ".env", used: len(workstation.Env) != 0, label: "environment"},
	}
	for _, field := range unsupported {
		if !field.used {
			continue
		}
		*targets = append(*targets, humanApprovalTarget(workstation, basePath+field.path, fmt.Sprintf("HUMAN_APPROVAL workstations must not declare %s", field.label)))
	}
}

func humanApprovalTarget(workstation factorydefinitions.FactoryWorkstationConfig, path, message string) Target {
	return Target{
		Code: CodeWorkstationHumanApproval, Severity: SeverityError, Message: message,
		Subject: Subject{Type: SubjectTypeWorkstation, ID: factorydefinitions.CanonicalFactoryGraphWorkstationID(workstation), Location: SubjectLocationDefinition},
		Path:    path,
	}
}

func nameValueErrorField(err error) string {
	if validationErr, ok := err.(*factorydefinitions.NameValueValidationError); ok && validationErr.Field != "" {
		return validationErr.Field
	}
	return "value"
}

// SubmittedTaxonomyCompatibilityTargets validates the exact public taxonomy
// observed before representation mapping normalizes compatibility aliases.
// The transport only copies these detached values; this service owns all
// compatibility decisions and target construction.
func SubmittedTaxonomyCompatibilityTargets(
	taxonomy factorydefinitions.SubmittedDefinitionTaxonomy,
) []Target {
	if len(taxonomy.Workers) == 0 || len(taxonomy.Workstations) == 0 {
		return nil
	}

	workersByName := make(map[string]factorydefinitions.SubmittedWorkerTaxonomy, len(taxonomy.Workers))
	for _, worker := range taxonomy.Workers {
		if name := strings.TrimSpace(worker.Name); name != "" {
			workersByName[name] = worker
		}
	}

	var targets []Target
	for _, workstation := range taxonomy.Workstations {
		worker, ok := workersByName[strings.TrimSpace(workstation.Worker)]
		if !ok || strings.TrimSpace(worker.Type) == "" ||
			submittedWorkerMatchesWorkstation(worker.Type, workstation) {
			continue
		}
		config := submittedWorkstationConfig(workstation)
		if _, ok := factorydefinitions.ExpectedWorkerBehaviorClassForWorkstation(
			factorydefinitions.Workstation{
				Name: config.Name, Type: config.Type, Kind: config.Kind,
				WorkerTypeName: config.WorkerTypeName,
			},
			worker.Type,
		); !ok {
			continue
		}
		targets = append(targets, Target{
			Code:     CodeWorkerWorkstationBehaviorCompatibility,
			Severity: SeverityError,
			Message: factorydefinitions.WorkerWorkstationBehaviorMismatchMessage(
				workstation.Name,
				submittedWorkstationTypeLabel(workstation),
				workstation.Behavior,
				worker.Name,
				worker.Type,
			),
			Subject: Subject{
				Type:     SubjectTypeWorkstation,
				ID:       factorydefinitions.CanonicalFactoryGraphWorkstationID(config),
				Location: SubjectLocationReference,
			},
			Path: fmt.Sprintf("factory.workstations[%d].worker", workstation.Index),
		})
	}
	return targets
}

func submittedWorkstationConfig(
	workstation factorydefinitions.SubmittedWorkstationTaxonomy,
) factorydefinitions.FactoryWorkstationConfig {
	return factorydefinitions.FactoryWorkstationConfig{
		Name: workstation.Name, Type: workstation.Type, Kind: workstation.Behavior,
		WorkerTypeName: workstation.Worker,
	}
}

func submittedWorkerMatchesWorkstation(
	workerType string,
	workstation factorydefinitions.SubmittedWorkstationTaxonomy,
) bool {
	config := submittedWorkstationConfig(workstation)
	value := factorydefinitions.Workstation{
		Name: config.Name, Type: config.Type, Kind: config.Kind,
		WorkerTypeName: config.WorkerTypeName,
	}
	if factorydefinitions.ExemptFromWorkerWorkstationCompatibility(value) {
		return true
	}
	workerType = strings.TrimSpace(workerType)
	if workerType == "" {
		return true
	}
	if config.Type == factorydefinitions.WorkstationTypeModel && workerType == factorydefinitions.WorkerTypeModel {
		return true
	}
	if config.Type == factorydefinitions.WorkstationTypeInvoke &&
		(workerType == factorydefinitions.WorkerTypeModel || workerType == factorydefinitions.WorkerTypeInference) {
		return true
	}
	if workerType == factorydefinitions.WorkerTypeModel {
		switch config.Type {
		case factorydefinitions.WorkstationTypeAgent, factorydefinitions.WorkstationTypeInference:
			return true
		}
	}
	return factorydefinitions.CompatibleWorkerWorkstationBehavior(workerType, config.Type, config.Kind)
}

func submittedWorkstationTypeLabel(workstation factorydefinitions.SubmittedWorkstationTaxonomy) string {
	if strings.TrimSpace(workstation.Type) != "" {
		return workstation.Type
	}
	if workstation.Behavior == factorydefinitions.WorkstationKindPoller {
		return factorydefinitions.WorkstationTypePoller
	}
	return factorydefinitions.WorkstationTypeModel
}

// PollerRunWorkstationKindTargets returns validation targets when an explicit
// POLLER_RUN workstation type conflicts with a non-poller workstation kind.
func PollerRunWorkstationKindTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg == nil || len(cfg.Workstations) == 0 {
		return nil
	}

	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		if factorydefinitions.StrictPublicFactoryWorkstationType(workstation.Type) != factorydefinitions.WorkstationTypePoller {
			continue
		}
		if workstation.Kind == "" || workstation.Kind == factorydefinitions.WorkstationKindPoller {
			continue
		}

		behaviorLabel := factorydefinitions.CanonicalPublicWorkstationKind(workstation.Kind)
		basePath := fmt.Sprintf("%s.workstations[%d](%s)", validationRoot, workstationIndex, workstation.Name)
		targets = append(targets, Target{
			Code:     CodePollerRunWorkstationKindMismatch,
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"workstation %q uses POLLER_RUN but behavior %q is not poller; set behavior to POLLER or choose a different workstation type",
				workstation.Name,
				behaviorLabel,
			),
			Subject: Subject{
				Type:     SubjectTypeWorkstation,
				ID:       workstation.Name,
				Location: SubjectLocationReference,
			},
			Path: basePath + ".behavior",
		})
	}

	return targets
}

package service

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// OrchestratorDefinitionValidation implements the Factory Definition port for
// runtime-owned JavaScript workflow and policy semantics.
type OrchestratorDefinitionValidation struct {
	workflows factoryruntime.JavaScriptWorkflowDefinitions
}

// NewOrchestratorDefinitionValidator returns the runtime-owned orchestrator
// validator injected into Factory Definition validation by Wire.
func NewOrchestratorDefinitionValidator(
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
) OrchestratorDefinitionValidation {
	return OrchestratorDefinitionValidation{workflows: workflows}
}

func (validation OrchestratorDefinitionValidation) ValidateJavaScriptFactoryDefinition(
	_ context.Context,
	config *factorydefinitions.FactoryOrchestratorJavaScriptConfig,
	reader factorydefinitions.WorkflowSourceReader,
) []factorydefinitions.ValidationTarget {
	if config == nil {
		return nil
	}
	if validation.workflows == nil {
		return []factorydefinitions.ValidationTarget{{
			Code:     "JAVASCRIPT_WORKFLOW_SERVICE_UNAVAILABLE",
			Severity: factorydefinitions.ValidationSeverityError,
			Message:  "JavaScript workflow validation service is unavailable",
			Path:     "factory.orchestrator.javascript",
			Subject: factorydefinitions.ValidationSubject{
				Type:     factorydefinitions.ValidationSubjectTypeFactory,
				ID:       "factory",
				Location: factorydefinitions.ValidationSubjectLocationDefinition,
			},
		}}
	}

	var targets []factorydefinitions.ValidationTarget
	targets = append(targets, javascriptWorkflowConfigAndInlineTargets(validation.workflows, config)...)
	targets = append(targets, javascriptWorkflowPolicyTargets(config)...)
	targets = append(targets, javascriptWorkflowSourceTargets(validation.workflows, config, reader)...)
	return targets
}

func javascriptWorkflowPolicyTargets(
	config *factorydefinitions.FactoryOrchestratorJavaScriptConfig,
) []factorydefinitions.ValidationTarget {
	resolution := factoryruntime.ResolveJavaScriptFactoryDefaultPolicy(config.DefaultPolicy)
	if len(resolution.Issues) == 0 {
		return nil
	}
	targets := make([]factorydefinitions.ValidationTarget, 0, len(resolution.Issues))
	for _, issue := range resolution.Issues {
		targetPath := "javascript.defaultPolicy"
		switch {
		case issue.Path == "orchestrator.javascript.defaultPolicy":
			targetPath = "javascript.defaultPolicy"
		case strings.HasPrefix(issue.Path, "policy."):
			targetPath = "javascript.defaultPolicy." + strings.TrimPrefix(issue.Path, "policy.")
		}
		targets = append(targets, factorydefinitions.ValidationTarget{
			Code:     issue.Code,
			Severity: factorydefinitions.ValidationSeverityError,
			Message:  issue.Message,
			Path:     "factory.orchestrator." + targetPath,
			Subject: factorydefinitions.ValidationSubject{
				Type:     factorydefinitions.ValidationSubjectTypeFactory,
				ID:       "factory",
				Location: factorydefinitions.ValidationSubjectLocationDefinition,
			},
		})
	}
	return targets
}

func javascriptWorkflowConfigAndInlineTargets(
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
	config *factorydefinitions.FactoryOrchestratorJavaScriptConfig,
) []factorydefinitions.ValidationTarget {
	var targets []factorydefinitions.ValidationTarget
	configResult := workflows.Validate(factoryruntime.WorkflowValidationRequest{
		ConfigPath: "orchestrator.javascript",
		Metadata:   config.Metadata,
		ArgsSchema: config.ArgsSchema,
	})
	targets = append(targets, workflowIssuesToDefinitionTargets(configResult.Issues)...)

	if config.InlineSource == nil {
		return targets
	}
	inline := strings.TrimSpace(config.InlineSource.Inline)
	if inline == "" {
		return targets
	}
	inlineResult := workflows.Validate(factoryruntime.WorkflowValidationRequest{
		Source:     inline,
		SourceRef:  "inline",
		ConfigPath: "orchestrator.javascript.inlineSource",
	})
	return append(targets, workflowIssuesToDefinitionTargets(inlineResult.Issues)...)
}

func javascriptWorkflowSourceTargets(
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
	config *factorydefinitions.FactoryOrchestratorJavaScriptConfig,
	reader factorydefinitions.WorkflowSourceReader,
) []factorydefinitions.ValidationTarget {
	if reader == nil {
		return nil
	}
	sourceRef := strings.TrimSpace(config.SourceRef)
	if sourceRef == "" {
		return nil
	}
	if config.InlineSource != nil && strings.TrimSpace(config.InlineSource.Inline) != "" {
		return nil
	}
	content, err := reader.ReadWorkflowSource(sourceRef)
	if err != nil {
		return []factorydefinitions.ValidationTarget{workflowIssueToDefinitionTarget(
			factoryruntime.WorkflowValidationIssue{
				Code:    factoryruntime.WorkflowValidationCodeSourceUnreadable,
				Message: fmt.Sprintf("unable to read workflow source %q: %v", sourceRef, err),
				Path:    "orchestrator.javascript.sourceRef",
			},
		)}
	}
	loaded, loadIssues := workflows.LoadSource(factoryruntime.WorkflowValidationLoadRequest{
		SourceRef: sourceRef,
		Content:   content,
	})
	if len(loadIssues) > 0 {
		return workflowIssuesToDefinitionTargets(loadIssues)
	}
	if expectedHash := strings.TrimSpace(config.SourceHash); expectedHash != "" && expectedHash != loaded.SourceHash {
		return []factorydefinitions.ValidationTarget{workflowIssueToDefinitionTarget(
			factoryruntime.WorkflowValidationIssue{
				Code:    factoryruntime.WorkflowValidationCodeSourceHashMismatch,
				Message: fmt.Sprintf("orchestrator.javascript.sourceHash %q does not match loaded workflow source hash %q", expectedHash, loaded.SourceHash),
				Path:    "orchestrator.javascript.sourceHash",
			},
		)}
	}
	fileResult := workflows.ValidateLoaded(loaded, factoryruntime.WorkflowValidationRequest{
		ConfigPath: "orchestrator.javascript.sourceRef",
		Metadata:   config.Metadata,
		ArgsSchema: config.ArgsSchema,
	})
	return workflowIssuesToDefinitionTargets(fileResult.Issues)
}

func workflowIssuesToDefinitionTargets(
	issues []factoryruntime.WorkflowValidationIssue,
) []factorydefinitions.ValidationTarget {
	if len(issues) == 0 {
		return nil
	}
	targets := make([]factorydefinitions.ValidationTarget, 0, len(issues))
	for _, issue := range issues {
		targets = append(targets, workflowIssueToDefinitionTarget(issue))
	}
	return targets
}

func workflowIssueToDefinitionTarget(
	issue factoryruntime.WorkflowValidationIssue,
) factorydefinitions.ValidationTarget {
	targetPath := "javascript"
	switch {
	case strings.HasPrefix(issue.Path, "orchestrator.javascript."):
		targetPath = strings.TrimPrefix(issue.Path, "orchestrator.")
	case issue.Path == "inline":
		targetPath = "javascript.inlineSource"
	case issue.Path != "":
		targetPath = "javascript.sourceRef"
	}
	return factorydefinitions.ValidationTarget{
		Code:     issue.Code,
		Severity: factorydefinitions.ValidationSeverityError,
		Message:  issue.Message + issue.LocationSuffix(),
		Path:     "factory." + targetPath,
		Subject: factorydefinitions.ValidationSubject{
			Type:     factorydefinitions.ValidationSubjectTypeFactory,
			ID:       "factory",
			Location: factorydefinitions.ValidationSubjectLocationDefinition,
		},
	}
}

var _ factorydefinitions.OrchestratorDefinitionValidator = OrchestratorDefinitionValidation{}

package validation

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func OrchestratorTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg == nil {
		return nil
	}
	if cfg.Orchestrator == nil {
		return nil
	}

	var targets []Target
	kind := strings.TrimSpace(cfg.Orchestrator.Kind)
	if kind == "" {
		return nil
	}

	canonicalKind := interfaces.StrictPublicFactoryOrchestratorKind(kind)
	if canonicalKind == "" {
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorUnsupportedKind,
			"kind",
			fmt.Sprintf("unsupported orchestrator.kind %q (supported: %q, %q)", kind, interfaces.OrchestratorKindPetri, interfaces.OrchestratorKindJavaScript),
		))
		return targets
	}

	switch canonicalKind {
	case interfaces.OrchestratorKindPetri:
		targets = append(targets, incompatibleJavaScriptOrchestratorTargets(cfg)...)
	case interfaces.OrchestratorKindJavaScript:
		targets = append(targets, incompatiblePetriOrchestratorTargets(cfg)...)
		targets = append(targets, javascriptOrchestratorConfigTargets(cfg)...)
	}
	return targets
}

func incompatiblePetriOrchestratorTargets(cfg *interfaces.FactoryConfig) []Target {
	var targets []Target
	if cfg.Orchestrator != nil && cfg.Orchestrator.Petri != nil {
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorIncompatiblePetriConfig,
			"petri",
			"orchestrator.petri is only valid when orchestrator.kind = PETRI",
		))
	}
	if len(cfg.WorkTypes) > 0 {
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorIncompatiblePetriField,
			"workTypes",
			"workTypes are only valid for orchestrator.kind = PETRI",
		))
	}
	if len(cfg.Workers) > 0 {
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorIncompatiblePetriField,
			"workers",
			"workers are only valid for orchestrator.kind = PETRI",
		))
	}
	if len(cfg.Workstations) > 0 {
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorIncompatiblePetriField,
			"workstations",
			"workstations are only valid for orchestrator.kind = PETRI",
		))
	}
	return targets
}

func incompatibleJavaScriptOrchestratorTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg.Orchestrator == nil || cfg.Orchestrator.JavaScript == nil {
		return nil
	}
	return []Target{orchestratorTarget(
		CodeOrchestratorIncompatibleJavaScriptConfig,
		"javascript",
		"orchestrator.javascript is only valid when orchestrator.kind = JAVASCRIPT",
	)}
}

func javascriptOrchestratorConfigTargets(cfg *interfaces.FactoryConfig) []Target {
	jsCfg := cfg.Orchestrator.JavaScript
	if jsCfg == nil {
		return []Target{orchestratorTarget(
			CodeOrchestratorJavaScriptMissingConfig,
			"javascript",
			"orchestrator.javascript is required when orchestrator.kind = JAVASCRIPT",
		)}
	}

	var targets []Target
	sourceRef := strings.TrimSpace(jsCfg.SourceRef)
	hasInline := jsCfg.InlineSource != nil && strings.TrimSpace(jsCfg.InlineSource.Inline) != ""
	switch {
	case sourceRef == "" && !hasInline:
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorJavaScriptMissingSource,
			"javascript.sourceRef",
			"JavaScript factories require orchestrator.javascript.sourceRef or orchestrator.javascript.inlineSource",
		))
	case sourceRef != "" && hasInline:
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorJavaScriptConflictingSource,
			"javascript.sourceRef",
			"JavaScript factories must declare either orchestrator.javascript.sourceRef or orchestrator.javascript.inlineSource, not both",
		))
	}
	if jsCfg.InlineSource != nil {
		encoding := strings.TrimSpace(jsCfg.InlineSource.Encoding)
		if encoding != "" && encoding != interfaces.OrchestratorInlineEncoding {
			targets = append(targets, orchestratorTarget(
				CodeOrchestratorJavaScriptInvalidInlineEncoding,
				"javascript.inlineSource.encoding",
				fmt.Sprintf("orchestrator.javascript.inlineSource.encoding must be %q when provided", interfaces.OrchestratorInlineEncoding),
			))
		}
	}
	return targets
}

func orchestratorTarget(code, path, message string) Target {
	return Target{
		Code:     code,
		Severity: SeverityError,
		Message:  message,
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       "factory",
			Location: SubjectLocationDefinition,
		},
		Path: fmt.Sprintf("%s.orchestrator.%s", validationRoot, path),
	}
}

// IsPetriOrchestratorValidationScope reports whether Petri graph validation should run.
func IsPetriOrchestratorValidationScope(cfg *interfaces.FactoryConfig) bool {
	return interfaces.IsPetriOrchestratorFactory(cfg)
}

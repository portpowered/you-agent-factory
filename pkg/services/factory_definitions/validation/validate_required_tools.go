package validation

import (
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
)

// ValidateDeclarativeRequiredTools runs declarative required-tool validation and
// returns Definition-owned targets for manifest shape defects and checker
// failures without requiring live host tool installation when checker is nil.
func ValidateDeclarativeRequiredTools(
	cfg *interfaces.FactoryConfig,
	checker RequiredToolChecker,
) Result {
	topologyResult := ValidateRequiredTools(cfg, checker)
	if topologyResult == nil || len(topologyResult.Findings) == 0 {
		return Result{}
	}
	return Result{Targets: requiredToolTargetsFromFindings(cfg, topologyResult.Findings)}
}

func requiredToolTargetsFromFindings(
	cfg *interfaces.FactoryConfig,
	findings []Finding,
) []Target {
	targets := make([]Target, 0, len(findings))
	for _, finding := range findings {
		targets = append(targets, requiredToolTargetFromFinding(cfg, finding))
	}
	return targets
}

func requiredToolTargetFromFinding(cfg *interfaces.FactoryConfig, finding Finding) Target {
	path := finding.Path
	if path != "" && !strings.HasPrefix(path, validationRoot+".") {
		path = validationRoot + "." + path
	}
	return Target{
		Code:     requiredToolCodeForRule(finding.Rule),
		Severity: finding.Severity,
		Message:  finding.Message,
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       requiredToolSubjectID(cfg, finding.Path),
			Location: SubjectLocationDefinition,
		},
		Path: path,
	}
}

func requiredToolCodeForRule(rule string) string {
	switch rule {
	case "required-tool-name":
		return CodeRequiredToolName
	case "required-tool-command":
		return CodeRequiredToolCommand
	case "required-tool-version-args":
		return CodeRequiredToolVersionArgs
	case "required-tool-missing":
		return CodeRequiredToolMissing
	case "required-tool-version-probe":
		return CodeRequiredToolVersionProbe
	default:
		return rule
	}
}

func requiredToolSubjectID(cfg *interfaces.FactoryConfig, path string) string {
	const prefix = "resourceManifest.requiredTools["
	if !strings.HasPrefix(path, prefix) {
		return "resourceManifest"
	}
	rest := strings.TrimPrefix(path, prefix)
	closeBracket := strings.Index(rest, "]")
	if closeBracket < 0 {
		return "resourceManifest"
	}
	indexText := rest[:closeBracket]
	var index int
	if _, err := fmt.Sscanf(indexText, "%d", &index); err != nil {
		return "resourceManifest"
	}
	if cfg == nil || cfg.ResourceManifest == nil || index < 0 || index >= len(cfg.ResourceManifest.RequiredTools) {
		return "resourceManifest"
	}
	name := strings.TrimSpace(cfg.ResourceManifest.RequiredTools[index].Name)
	if name == "" {
		return fmt.Sprintf("requiredTools[%d]", index)
	}
	return name
}

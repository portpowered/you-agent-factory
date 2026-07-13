package goal

import (
	"fmt"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/definitions/goal"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// PackagedGoalPromptDriftError reports canonical-vs-derived prompt mismatch for one role.
type PackagedGoalPromptDriftError struct {
	Role string
}

func (e PackagedGoalPromptDriftError) Error() string {
	return fmt.Sprintf("packaged goal prompt drift for role %q: derived prompt does not match canonical packaged source", e.Role)
}

// CheckPackagedGoalMaterializedPromptDrift returns an error when any materialized
// on-disk prompt for a packaged goal role does not match the canonical authored source.
func CheckPackagedGoalMaterializedPromptDrift(factoryDir string) error {
	for _, source := range PackagedGoalRolePromptSources {
		authoredPrompt, ok := builtingoal.AuthoredRolePrompt(source.Role)
		if !ok {
			return fmt.Errorf("missing canonical authored prompt for role %q", source.Role)
		}

		derivedPrompt, err := loadPackagedGoalRolePrompt(factoryDir, source)
		if err != nil {
			return fmt.Errorf("load derived prompt for role %q: %w", source.Role, err)
		}
		if derivedPrompt != authoredPrompt {
			return PackagedGoalPromptDriftError{Role: source.Role}
		}
	}
	return nil
}

// CheckPackagedGoalAssembledPromptDrift returns an error when any prompt body in
// the assembled built-in factory JSON does not match the canonical authored source.
func CheckPackagedGoalAssembledPromptDrift() error {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(builtingoal.BuiltInGoalFactoryJSON)
	if err != nil {
		return fmt.Errorf("load assembled built-in goal factory: %w", err)
	}

	for _, source := range PackagedGoalRolePromptSources {
		authoredPrompt, ok := builtingoal.AuthoredRolePrompt(source.Role)
		if !ok {
			return fmt.Errorf("missing canonical authored prompt for role %q", source.Role)
		}

		derivedPrompt, err := assembledPackagedGoalRolePrompt(cfg, source)
		if err != nil {
			return fmt.Errorf("load assembled prompt for role %q: %w", source.Role, err)
		}
		if derivedPrompt != authoredPrompt {
			return PackagedGoalPromptDriftError{Role: source.Role}
		}
	}
	return nil
}

func assembledPackagedGoalRolePrompt(cfg *interfaces.FactoryConfig, source PackagedGoalRolePromptSource) (string, error) {
	switch source.SourceKind {
	case PackagedGoalRolePromptSourceKindWorkerBody:
		for _, worker := range cfg.Workers {
			if worker.Name == source.WorkerName {
				return strings.TrimSpace(worker.Body), nil
			}
		}
		return "", fmt.Errorf("missing worker %q", source.WorkerName)
	default:
		for _, workstation := range cfg.Workstations {
			if workstation.Name == source.WorkstationName {
				return strings.TrimSpace(workstation.Body), nil
			}
		}
		return "", fmt.Errorf("missing workstation %q", source.WorkstationName)
	}
}

package goal

import (
	"encoding/json"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	builtingoal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/goal"
)

type packagedGoalPromptConfig struct {
	Workers []struct {
		Name string `json:"name"`
		Body string `json:"body"`
	} `json:"workers"`
	Workstations []struct {
		Name string `json:"name"`
		Body string `json:"body"`
	} `json:"workstations"`
}

// PackagedGoalPromptDriftError reports canonical-vs-derived prompt mismatch for one role.
type PackagedGoalPromptDriftError struct {
	Role string
}

func (e PackagedGoalPromptDriftError) Error() string {
	return fmt.Sprintf("packaged goal prompt drift for role %q: derived prompt does not match canonical packaged source", e.Role)
}

// CheckPackagedGoalMaterializedPromptDrift returns an error when any materialized
// on-disk prompt for a packaged goal role does not match the canonical authored source.
func CheckPackagedGoalMaterializedPromptDrift(
	fileSystem factorydefinitions.PackagedGoalPromptFileSystem,
	factoryDir string,
) error {
	if fileSystem == nil {
		return fmt.Errorf("packaged Goal prompt filesystem is required")
	}
	canonical, err := decodePackagedGoalPromptConfig()
	if err != nil {
		return fmt.Errorf("load assembled built-in goal factory: %w", err)
	}
	for _, source := range PackagedGoalRolePromptSources {
		authoredPrompt, err := assembledPackagedGoalRolePrompt(canonical, source)
		if err != nil {
			return fmt.Errorf("load canonical prompt for role %q: %w", source.Role, err)
		}

		derivedPrompt, err := loadPackagedGoalRolePrompt(fileSystem, factoryDir, source)
		if err != nil {
			return fmt.Errorf("load derived prompt for role %q: %w", source.Role, err)
		}
		if derivedPrompt != authoredPrompt {
			return PackagedGoalPromptDriftError{Role: source.Role}
		}
	}
	return nil
}

// CheckPackagedGoalAssembledPromptDrift verifies that every declared goal role
// resolves to a non-empty prompt in the shared assembler's canonical payload.
func CheckPackagedGoalAssembledPromptDrift() error {
	canonical, err := decodePackagedGoalPromptConfig()
	if err != nil {
		return fmt.Errorf("load assembled built-in goal factory: %w", err)
	}
	for _, source := range PackagedGoalRolePromptSources {
		prompt, err := assembledPackagedGoalRolePrompt(canonical, source)
		if err != nil {
			return fmt.Errorf("load assembled prompt for role %q: %w", source.Role, err)
		}
		if strings.TrimSpace(prompt) == "" {
			return fmt.Errorf("assembled prompt for role %q is empty", source.Role)
		}
	}
	return nil
}

func decodePackagedGoalPromptConfig() (*packagedGoalPromptConfig, error) {
	var config packagedGoalPromptConfig
	if err := json.Unmarshal(builtingoal.BuiltInGoalFactoryJSON, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func assembledPackagedGoalRolePrompt(cfg *packagedGoalPromptConfig, source PackagedGoalRolePromptSource) (string, error) {
	switch source.SourceKind {
	case PackagedGoalRolePromptSourceKindWorkerBody:
		for _, worker := range cfg.Workers {
			if worker.Name == source.WorkerName {
				return worker.Body, nil
			}
		}
		return "", fmt.Errorf("missing worker %q", source.WorkerName)
	default:
		for _, workstation := range cfg.Workstations {
			if workstation.Name == source.WorkstationName {
				return workstation.Body, nil
			}
		}
		return "", fmt.Errorf("missing workstation %q", source.WorkstationName)
	}
}

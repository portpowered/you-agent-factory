package goal

import (
	"fmt"
	"path/filepath"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func packagedGoalMaterializedPromptPath(factoryDir string, source PackagedGoalRolePromptSource) string {
	if source.SourceKind == PackagedGoalRolePromptSourceKindWorkerBody {
		return filepath.Join(factoryDir, factorydefinitions.WorkersDir, source.WorkerName, factorydefinitions.FactoryAgentsFileName)
	}
	return filepath.Join(factoryDir, factorydefinitions.WorkstationsDir, source.WorkstationName, source.PromptFile)
}

func loadPackagedGoalRolePrompt(
	fileSystem factorydefinitions.PackagedGoalPromptFileSystem,
	factoryDir string,
	source PackagedGoalRolePromptSource,
) (string, error) {
	if fileSystem == nil {
		return "", fmt.Errorf("packaged Goal prompt filesystem is required")
	}
	promptPath := packagedGoalMaterializedPromptPath(factoryDir, source)
	data, err := fileSystem.ReadFile(promptPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

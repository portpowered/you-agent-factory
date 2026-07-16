package goal

import (
	"os"
	"path/filepath"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

func packagedGoalMaterializedPromptPath(factoryDir string, source PackagedGoalRolePromptSource) string {
	if source.SourceKind == PackagedGoalRolePromptSourceKindWorkerBody {
		return filepath.Join(factoryDir, interfaces.WorkersDir, source.WorkerName, interfaces.FactoryAgentsFileName)
	}
	return filepath.Join(factoryDir, interfaces.WorkstationsDir, source.WorkstationName, source.PromptFile)
}

func loadPackagedGoalRolePrompt(factoryDir string, source PackagedGoalRolePromptSource) (string, error) {
	promptPath := packagedGoalMaterializedPromptPath(factoryDir, source)
	data, err := os.ReadFile(promptPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

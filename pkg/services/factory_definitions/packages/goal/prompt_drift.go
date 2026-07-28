package goal

import (
	distributiongoal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/goal"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// PackagedGoalPromptDriftError reports canonical-vs-derived prompt mismatch for one role.
type PackagedGoalPromptDriftError = distributiongoal.PackagedGoalPromptDriftError

// CheckPackagedGoalMaterializedPromptDrift returns an error when any materialized
// on-disk prompt for a packaged goal role does not match the canonical authored source.
func CheckPackagedGoalMaterializedPromptDrift(
	fileSystem factorydefinitions.PackagedGoalPromptFileSystem,
	factoryDir string,
) error {
	return distributiongoal.CheckPackagedGoalMaterializedPromptDrift(fileSystem, factoryDir)
}

// CheckPackagedGoalAssembledPromptDrift verifies that every declared goal role
// resolves to a non-empty prompt in the shared assembler's canonical payload.
func CheckPackagedGoalAssembledPromptDrift() error {
	return distributiongoal.CheckPackagedGoalAssembledPromptDrift()
}

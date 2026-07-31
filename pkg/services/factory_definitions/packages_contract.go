package factorydefinitions

import (
	"io/fs"
	"strings"

	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
	distributionpackageassets "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packageassets"
)

type PackagedDefinition = contracts.PackagedDefinition
type PackagedFactoryFormat = contracts.PackagedFactoryFormat

const (
	PackagedFactoryFormatJSON = contracts.PackagedFactoryFormatJSON
	PackagedFactoryFormatYAML = contracts.PackagedFactoryFormatYAML
	PackagedFactoryFormatYML  = contracts.PackagedFactoryFormatYML
)

// PackagedGoalPromptFileSystem is the exact filesystem effect used by the
// packaged Goal drift check to read one already-resolved prompt path.
type PackagedGoalPromptFileSystem interface {
	ReadFile(string) ([]byte, error)
}

const (
	PackagedDeepResearchFactoryName      = "@you/deep-research"
	PackagedFusionFactoryName            = "@you/fusion"
	PackagedGoalFactoryName              = "@you/goal"
	PackagedGoalWorkTypeName             = "goal"
	PackagedGoalExecuteWorkstationName   = "execute-goal"
	PackagedGoalPlanWorkstationName      = "plan-goal"
	PackagedGoalCheckWorkstationName     = "check-goal"
	PackagedGoalReviewWorkstationName    = "review-goal"
	PackagedReviewFactoryName            = "@you/review"
	PackagedReviewExecuteWorkstationName = "execute-review-work"
	PackagedReviewWorkstationName        = "review-review-work"
	PackagedTournamentFactoryName        = "@you/tournament"
	PackagedSpawnFactoryName             = "@you/spawn"
	PackagedLoopFactoryName              = "@you/loop"
	PackagedPlanExecuteFactoryName       = "@you/plan-execute"
	PackagedPlanParallelFactoryName      = "@you/plan-parallel"
	PackagedClassifyFactoryName          = "@you/classify"
	PackagedFullFlowFactoryName          = "@you/full-flow"
)

// CustomerVisibleFactoryName returns the customer-facing Factory identifier for
// diagnostics when runtime configs use authored or generated short names.
func CustomerVisibleFactoryName(cfg *FactoryConfig) string {
	if cfg == nil {
		return ""
	}
	name := strings.TrimSpace(cfg.Name)
	if strings.HasPrefix(name, "@you/") {
		return name
	}
	project := strings.TrimSpace(cfg.Project)
	if strings.HasPrefix(project, "builtin-") {
		return "@you/" + strings.TrimPrefix(project, "builtin-")
	}
	return name
}

// PackagedFactoryAssetDefinition describes one authored packaged Factory and
// the assets available beneath its package-owned asset root.
type PackagedFactoryAssetDefinition = distributionpackageassets.Definition

// AssemblePackagedFactoryAssets resolves package-owned assets and returns a
// canonical JSON payload without persisting or installing the definition.
func AssemblePackagedFactoryAssets(definition PackagedFactoryAssetDefinition) ([]byte, error) {
	return distributionpackageassets.Assemble(definition)
}

// PackagedFactoryAssetFileSystem is the exact filesystem effect used when
// assembling packaged Factory assets from an authored package directory.
type PackagedFactoryAssetFileSystem = fs.FS

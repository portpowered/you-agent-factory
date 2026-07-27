package factorydefinitions

import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"

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
	PackagedReviewFactoryName            = "@you/review"
	PackagedReviewExecuteWorkstationName = "execute-review-work"
	PackagedReviewWorkstationName        = "review-review-work"
)

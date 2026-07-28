package factorydefinitions

import (
	"strings"

	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
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
	PackagedReviewFactoryName            = "@you/review"
	PackagedReviewExecuteWorkstationName = "execute-review-work"
	PackagedReviewWorkstationName        = "review-review-work"
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

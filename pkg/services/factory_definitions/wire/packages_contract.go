package wire

import (
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionassets "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire/assets"
)

// CustomerVisibleFactoryName returns the customer-facing Factory identifier
// for diagnostics when runtime configs use authored or generated short names.
func CustomerVisibleFactoryName(cfg *factorydefinitions.FactoryConfig) string {
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

// AssemblePackagedFactoryAssets resolves package-owned assets and returns a
// canonical JSON payload without persisting or installing the definition.
func AssemblePackagedFactoryAssets(
	definition factorydefinitions.PackagedFactoryAssetDefinition,
) ([]byte, error) {
	return distributionassets.AssemblePackagedFactoryAssets(definition)
}

package validation

import (
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// ValidateInstallPackagedFactoryRequest rejects incompatible distribute inputs
// before catalog resolution or filesystem effects begin.
func ValidateInstallPackagedFactoryRequest(
	request factorydefinitions.InstallPackagedFactoryRequest,
) error {
	if hasScaffoldSpecificOptions(request.Scaffold) {
		return factorydefinitions.ErrIncompatibleFactoryDistributeOptions
	}
	return nil
}

func hasScaffoldSpecificOptions(request factorydefinitions.CreateFactoryScaffoldRequest) bool {
	return strings.TrimSpace(request.TargetDir) != "" ||
		strings.TrimSpace(request.Type) != "" ||
		strings.TrimSpace(request.Executor) != ""
}

// ValidateCreateFactoryScaffoldRequest rejects unsupported scaffold inputs
// before filesystem effects begin.
func ValidateCreateFactoryScaffoldRequest(
	request factorydefinitions.CreateFactoryScaffoldRequest,
) error {
	if strings.TrimSpace(request.TargetDir) == "" {
		return factorydefinitions.ErrFactoryDistributeFailed
	}
	scaffoldType := strings.TrimSpace(request.Type)
	if scaffoldType != "" && scaffoldType != factorydefinitions.DefaultScaffoldType {
		return factorydefinitions.ErrFactoryDistributeFailed
	}
	return nil
}

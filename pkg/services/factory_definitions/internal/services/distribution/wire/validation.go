package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/internal/validation"
)

// ValidateInstallPackagedFactoryRequest exposes the distribution-owned pure
// request policy to the owner composition boundary.
func ValidateInstallPackagedFactoryRequest(
	request factorydefinitions.InstallPackagedFactoryRequest,
) error {
	return distributionvalidation.ValidateInstallPackagedFactoryRequest(request)
}

// ValidateCreateFactoryScaffoldRequest exposes the distribution-owned pure
// scaffold policy to the owner composition boundary.
func ValidateCreateFactoryScaffoldRequest(
	request factorydefinitions.CreateFactoryScaffoldRequest,
) error {
	return distributionvalidation.ValidateCreateFactoryScaffoldRequest(request)
}

package factorydefinitions

import (
	"fmt"
	"strings"
)

// ErrIncompatibleFactoryDistributeOptions reports that packaged selection and
// scaffold-specific inputs cannot be combined in one distribute request.
var ErrIncompatibleFactoryDistributeOptions = fmt.Errorf(
	"packaged factory selection cannot be combined with scaffold-specific options",
)

// ValidateInstallPackagedFactoryRequest rejects incompatible distribute inputs
// before catalog resolution or filesystem effects begin.
func ValidateInstallPackagedFactoryRequest(request InstallPackagedFactoryRequest) error {
	if hasScaffoldSpecificOptions(request.Scaffold) {
		return ErrIncompatibleFactoryDistributeOptions
	}
	return nil
}

func hasScaffoldSpecificOptions(request CreateFactoryScaffoldRequest) bool {
	return strings.TrimSpace(request.TargetDir) != "" ||
		strings.TrimSpace(request.Type) != "" ||
		strings.TrimSpace(request.Executor) != ""
}

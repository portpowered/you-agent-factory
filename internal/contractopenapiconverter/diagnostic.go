package contractopenapiconverter

import (
	"fmt"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

const documentRoot = "schema"

func unsupportedKeyword(key, path string) contractvalidator.Diagnostic {
	instancePath := path
	if instancePath == "" {
		instancePath = "/"
	}
	return contractvalidator.Diagnostic{
		Code:     codeUnsupportedKeyword,
		Path:     instancePath,
		Message:  fmt.Sprintf("keyword %q is outside the %s converter profile", key, profileStageCoreShapes),
		Document: documentRoot,
	}
}

func invalidSchemaValue(path string) contractvalidator.Diagnostic {
	instancePath := path
	if instancePath == "" {
		instancePath = "/"
	}
	return contractvalidator.Diagnostic{
		Code:     codeUnsupportedKeyword,
		Path:     instancePath,
		Message:  "schema value must be an object",
		Document: documentRoot,
	}
}

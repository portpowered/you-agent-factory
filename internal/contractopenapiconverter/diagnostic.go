package contractopenapiconverter

import (
	"fmt"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

const documentRoot = "schema"

func unsupportedKeyword(key, path, stage string) contractvalidator.Diagnostic {
	instancePath := path
	if instancePath == "" {
		instancePath = "/"
	}
	return contractvalidator.Diagnostic{
		Code:     codeUnsupportedKeyword,
		Path:     instancePath,
		Message:  fmt.Sprintf("keyword %q is outside the %s converter profile", key, stage),
		Document: documentRoot,
	}
}

func unsupportedReference(reference, path string) contractvalidator.Diagnostic {
	instancePath := path
	if instancePath == "" {
		instancePath = "/"
	}
	return contractvalidator.Diagnostic{
		Code:     codeUnsupportedRef,
		Path:     instancePath,
		Message:  fmt.Sprintf("reference %q is outside the %s converter profile", reference, profileStageRefs),
		Document: documentRoot,
	}
}

func missingComponent(name string) contractvalidator.Diagnostic {
	return contractvalidator.Diagnostic{
		Code:     codeMissingComponent,
		Path:     "/$ref",
		Message:  fmt.Sprintf("component schema %q is not defined", name),
		Document: documentRoot,
	}
}

func referenceCycle(name string) contractvalidator.Diagnostic {
	return contractvalidator.Diagnostic{
		Code:     codeReferenceCycle,
		Path:     "/$defs/" + name,
		Message:  fmt.Sprintf("component schema %q participates in a reference cycle", name),
		Document: documentRoot,
	}
}

func invalidReference(path string) contractvalidator.Diagnostic {
	instancePath := path
	if instancePath == "" {
		instancePath = "/"
	}
	return contractvalidator.Diagnostic{
		Code:     codeInvalidReference,
		Path:     instancePath,
		Message:  "reference must be a non-empty string",
		Document: documentRoot,
	}
}

func refWithSiblingKeywords(path string) contractvalidator.Diagnostic {
	instancePath := path
	if instancePath == "" {
		instancePath = "/"
	}
	return contractvalidator.Diagnostic{
		Code:     codeUnsupportedRef,
		Path:     instancePath,
		Message:  fmt.Sprintf("$ref must be the only keyword in the %s converter profile", profileStageRefs),
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

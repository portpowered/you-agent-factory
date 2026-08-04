package contractopenapiconverter

const (
	profileStageCoreShapes          = "core-shapes"
	profileStageRefs                = "refs"
	profileStageCompositionNullable = "composition-nullable"
	profileStageFailClosed          = "fail-closed"
	codeUnsupportedKeyword          = "openapi.convert.unsupported_keyword"
	codeUnsupportedRef              = "openapi.convert.unsupported_reference"
	codeMissingComponent            = "openapi.convert.missing_component"
	codeReferenceCycle              = "openapi.convert.reference_cycle"
	codeInvalidReference            = "openapi.convert.invalid_reference"
	codeAmbiguousNullable           = "openapi.convert.ambiguous_nullable"
	codeAmbiguousDiscriminator      = "openapi.convert.ambiguous_discriminator"
	codeAmbiguousComposition        = "openapi.convert.ambiguous_composition"
	codeAmbiguousDefault            = "openapi.convert.ambiguous_default"
)

var (
	supportedPrimitiveTypes = map[string]struct{}{
		"string":  {},
		"number":  {},
		"integer": {},
		"boolean": {},
		"object":  {},
		"array":   {},
	}

	coreShapeKeywords = map[string]struct{}{
		"type":                 {},
		"format":               {},
		"enum":                 {},
		"properties":           {},
		"required":             {},
		"additionalProperties": {},
		"items":                {},
		"description":          {},
		"title":                {},
		"default":              {},
		"minimum":              {},
		"maximum":              {},
		"exclusiveMinimum":     {},
		"exclusiveMaximum":     {},
		"minLength":            {},
		"maxLength":            {},
		"pattern":              {},
		"minItems":             {},
		"maxItems":             {},
		"uniqueItems":          {},
	}
)

var compositionKeywords = map[string]struct{}{
	"allOf": {},
	"oneOf": {},
	"anyOf": {},
}

const negationKeyword = "not"

func isCoreShapeKeyword(key string) bool {
	if len(key) > 2 && key[0] == 'x' && key[1] == '-' {
		return false
	}
	_, ok := coreShapeKeywords[key]
	return ok
}

func isKeywordAllowed(stage, key string) bool {
	if isCoreShapeKeyword(key) {
		return true
	}
	if stage == profileStageCompositionNullable || stage == profileStageFailClosed {
		if key == "nullable" {
			return true
		}
		if _, ok := compositionKeywords[key]; ok {
			return true
		}
	}
	if stage == profileStageFailClosed && key == negationKeyword {
		return true
	}
	if stage == profileStageFailClosed && key == "$ref" {
		return true
	}
	if stage == profileStageRefs && key == "$ref" {
		return true
	}
	return false
}

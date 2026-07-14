package contractopenapiconverter

const (
	profileStageCoreShapes = "core-shapes"
	codeUnsupportedKeyword = "openapi.convert.unsupported_keyword"
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

func isCoreShapeKeyword(key string) bool {
	if len(key) > 2 && key[0] == 'x' && key[1] == '-' {
		return false
	}
	_, ok := coreShapeKeywords[key]
	return ok
}

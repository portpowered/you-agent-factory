// Package contractopenapidiff compares OpenAPI documents and classifies
// supported semantic differences for independent package semver decisions.
// It is build tooling and must not be imported by runtime packages.
package contractopenapidiff

// Classification is the aggregate semver signal for a compared document pair.
type Classification string

const (
	ClassificationMajor Classification = "major"
	ClassificationMinor Classification = "minor"
	ClassificationPatch Classification = "patch"
)

// Change is one classified OpenAPI difference with a stable code and location.
type Change struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

// Result is the comparator output for a successfully classified comparison.
type Result struct {
	Classification Classification `json:"classification"`
	Changes        []Change       `json:"changes"`
}

// Stable documentation-only change codes.
const (
	CodeInfoDescriptionChanged        = "openapi.doc.info.description"
	CodeInfoSummaryChanged            = "openapi.doc.info.summary"
	CodeInfoExternalDocsChanged       = "openapi.doc.info.external_docs"
	CodeOperationSummaryChanged       = "openapi.doc.operation.summary"
	CodeOperationDescriptionChanged   = "openapi.doc.operation.description"
	CodeOperationExternalDocsChanged  = "openapi.doc.operation.external_docs"
	CodeParameterDescriptionChanged   = "openapi.doc.parameter.description"
	CodeRequestBodyDescriptionChanged = "openapi.doc.request_body.description"
	CodeResponseDescriptionChanged    = "openapi.doc.response.description"
	CodeSchemaTitleChanged            = "openapi.doc.schema.title"
	CodeSchemaDescriptionChanged      = "openapi.doc.schema.description"
	CodeSchemaExternalDocsChanged     = "openapi.doc.schema.external_docs"
	CodeTagDescriptionChanged         = "openapi.doc.tag.description"
	CodeTagExternalDocsChanged        = "openapi.doc.tag.external_docs"
)

// Stable compatible-addition change codes.
const (
	CodeOperationAdded      = "openapi.add.operation"
	CodeParameterAdded      = "openapi.add.parameter"
	CodeSchemaAdded         = "openapi.add.schema"
	CodeSchemaPropertyAdded = "openapi.add.schema.property"
	CodeEnumValueAdded      = "openapi.add.enum.value"
)

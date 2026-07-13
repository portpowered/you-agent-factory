// Package openapitests documents Factory/OpenAPI config parity evidence and
// exercises cross-dialect agreement through focused table-driven tests.
package openapitests

// ParityInventoryFormatVersion identifies the Factory/OpenAPI parity index shape.
const ParityInventoryFormatVersion = "factory-openapi-parity/v1"

// ParityIndexBaselineRelativePath is the committed Factory/OpenAPI parity index fixture.
const ParityIndexBaselineRelativePath = "pkg/config/openapitests/testdata/baseline/factory-openapi-parity-index.json"

const (
	outcomeAccept = "accept"
	outcomeReject = "reject"

	entrypointGeneratedFactory = "GeneratedFactoryFromOpenAPIJSON"
	entrypointFactoryConfig    = "FactoryConfigFromOpenAPIJSON"

	categoryBoundaryEnum    = "boundary-enum"
	categoryGuardUnion      = "guard-union"
	categoryLayoutContract  = "layout-contract"
	categoryRetiredBoundary = "retired-boundary"
	categoryShapeMapping    = "shape-mapping"
	categoryTaxonomyEnum    = "taxonomy-enum"

	shapeGuard        = "guard"
	shapeLayout       = "layout"
	shapeOrchestrator = "orchestrator"
	shapeResource     = "resource"
	shapeWorker       = "worker"
	shapeWorkstation  = "workstation"
)

// ParityInventory indexes deterministic Factory/OpenAPI inputs and expected
// cross-dialect loader outcomes.
type ParityInventory struct {
	FormatVersion string       `json:"formatVersion"`
	Scope         string       `json:"scope"`
	Cases         []ParityCase `json:"cases"`
}

// ParityCase records one indexed input and the API/config-loader outcomes it documents.
type ParityCase struct {
	ID                    string   `json:"id"`
	Shape                 string   `json:"shape"`
	Category              string   `json:"category"`
	SourceTest            string   `json:"sourceTest"`
	Fixture               string   `json:"fixture"`
	Description           string   `json:"description"`
	APIOutcome            string   `json:"apiOutcome"`
	LoaderOutcome         string   `json:"loaderOutcome"`
	ExpectedErrorPath     string   `json:"expectedErrorPath,omitempty"`
	ExpectedErrorCategory string   `json:"expectedErrorCategory,omitempty"`
	ErrorFragments        []string `json:"errorFragments,omitempty"`
}

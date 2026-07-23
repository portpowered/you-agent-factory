// Package openapitests documents Factory/OpenAPI config parity evidence and
// exercises cross-dialect agreement through focused table-driven tests.
package openapitests

// ParityInventoryFormatVersion identifies the Factory/OpenAPI parity index shape.
const ParityInventoryFormatVersion = "factory-openapi-parity/v1"

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

// ProjectParityInventory builds the deterministic Factory/OpenAPI parity index
// from committed fixtures and documented API/config-loader outcomes.
func ProjectParityInventory() ParityInventory {
	cases := make([]ParityCase, 0, 20)
	cases = append(cases, baselineAcceptParityCases()...)
	cases = append(cases, baselineRejectParityCases()...)

	return ParityInventory{
		FormatVersion: ParityInventoryFormatVersion,
		Scope: "Factory/OpenAPI/projected-schema config parity index referencing existing openapitests " +
			"fixtures; each case records GeneratedFactoryFromOpenAPIJSON, FactoryConfigFromOpenAPIJSON, " +
			"and projected Draft 2020-12 factory.schema.json outcomes without changing schemas or mapping behavior",
		Cases: cases,
	}
}

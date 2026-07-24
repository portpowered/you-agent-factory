// Package functionaltestviz assembles and renders the functional-test Markdown
// catalog consumed by maintainers and later Make/CI wiring (FND-005).
//
// Coverage input is the gocoveragecheck coverage-summary JSON artifact only.
// This package does not parse coverage profiles (.out).
//
// Golden-backed rows require AttachGoldenProvenance (or equivalent fixture
// setup) before RenderCatalogMarkdown; missing manifests fail closed.
// Package coverage is rendered from CoverageSummary JSON fields only.
package functionaltestviz

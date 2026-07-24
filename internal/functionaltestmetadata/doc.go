// Package functionaltestmetadata inventories top-level Test* declarations under
// a functional-test source tree using the Go AST.
//
// Records include file-level build-constraint expressions and explicit golden
// fixture/manifest references when declared. Golden references are read from:
//   - a //golden: <path> directive in the test's doc comment, or
//   - a test-owned const/var string named golden, goldenManifest, goldenFixture,
//     Golden, GoldenManifest, or GoldenFixture inside the Test* body.
//
// Classification separates customer scenarios from harness verification:
//   - paths under internal/** are ClassificationHarness
//   - *_test.go basenames containing "helpers" are ClassificationHarness
//     (helper-only / shared-helper verification, including files with no Test*)
//   - all other inventoried Test* records are ClassificationCustomer
//
// CustomerScenarioCount equals the number of ClassificationCustomer records.
// Harness records remain in the inventory so later report rendering can mention
// them without mixing them into customer totals.
//
// Undocumented customer Test* identities are frozen in an exact deletion-only
// baseline (see Baseline, CheckAgainstBaseline, and ValidateBaselineUpdate).
// Helper-only and internal/** undocumented helpers never enter that ledger.
// CheckAgainstBaseline succeeds when the current undocumented customer set is
// identical to or a strict subset of the baseline; newly undocumented customer
// tests absent from the baseline fail closed with an actionable identity.
// ValidateBaselineUpdate rejects baseline expansions.
//
// Later foundation cells consume this package for catalog generation and CI
// enforcement. This package stays side-effect free aside from reading source
// and baseline files.
package functionaltestmetadata

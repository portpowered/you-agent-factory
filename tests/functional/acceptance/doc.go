// Package acceptance owns hermetic built-CLI functional acceptance scenarios
// for the S24 cross-surface matrix.
//
// Scenario-to-outcome mapping lives in internal/builtcliacceptance.S24Scenarios
// and is locked by TestS24ScenarioMatrix_EveryDocumentedScenarioHasFocusedAcceptanceTest.
// PR verification runs this package through make test-built-cli-acceptance.
package acceptance

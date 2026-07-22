// Package mockworkers loads and validates the mock-workers JSON contract. The
// input inventory types in this file document accepted authoring variants and
// loader outcomes without changing parse or validation behavior.
package mockworkers

// InputInventoryFormatVersion identifies the mock-worker input inventory shape.
const InputInventoryFormatVersion = "mock-workers-input/v1"

// InputIndexBaselineRelativePath is the committed mock-worker input index fixture.
const InputIndexBaselineRelativePath = "pkg/services/workers/interface/testdata/baseline/mock-workers-input-index.json"

const (
	outcomeAccept = "accept"
	outcomeReject = "reject"

	entrypointParseMockWorkersConfig = "ParseMockWorkersConfig"
	entrypointLoadMockWorkersConfig  = "LoadMockWorkersConfig"

	categoryParseEmptyDefault    = "parse-empty-default"
	categoryParseAcceptEntry     = "parse-accept-entry"
	categoryParseRejectEntry     = "parse-reject-entry"
	categoryParseScriptEntry     = "parse-script-entry"
	categoryParseUnmatchedPolicy = "parse-unmatched-policy"
	categoryParseDocsExample     = "parse-docs-example"
	categoryParseUnknownField    = "parse-unknown-field"
	categoryLoadFile             = "load-file"
	categoryLoadEmptyPath        = "load-empty-path"
)

// InputInventory indexes deterministic mock-worker inputs and expected loader outcomes.
type InputInventory struct {
	FormatVersion      string      `json:"formatVersion"`
	UnknownFieldPolicy string      `json:"unknownFieldPolicy"`
	LoaderEntrypoints  []string    `json:"loaderEntrypoints"`
	Cases              []InputCase `json:"cases"`
}

// InputCase records one indexed input and the production loader outcome it documents.
type InputCase struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Entrypoint  string `json:"entrypoint"`
	Outcome     string `json:"outcome"`
	Fixture     string `json:"fixture,omitempty"`
	Description string `json:"description"`

	ExpectedConfig *MockWorkersConfigExpectation `json:"expectedConfig,omitempty"`
	ErrorFragments []string                      `json:"errorFragments,omitempty"`
}

// MockWorkersConfigExpectation records expected parse/load outputs for accepted inputs.
type MockWorkersConfigExpectation struct {
	UnmatchedDispatchPolicy string                  `json:"unmatchedDispatchPolicy,omitempty"`
	MockWorkerCount         int                     `json:"mockWorkerCount,omitempty"`
	MockWorkers             []MockWorkerExpectation `json:"mockWorkers,omitempty"`
}

// MockWorkerExpectation records selective mockWorkers[] fields asserted by inventory tests.
type MockWorkerExpectation struct {
	ID              string `json:"id,omitempty"`
	WorkerName      string `json:"workerName,omitempty"`
	WorkstationName string `json:"workstationName,omitempty"`
	RunType         string `json:"runType,omitempty"`
	ScriptCommand   string `json:"scriptCommand,omitempty"`
	RejectExitCode  *int   `json:"rejectExitCode,omitempty"`
}

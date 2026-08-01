package operatorsettings

// InputInventoryFormatVersion identifies the operator config input inventory shape.
const InputInventoryFormatVersion = "operator-config-input/v1"

// InputIndexBaselineRelativePath is the committed operator config input index fixture.
const InputIndexBaselineRelativePath = "pkg/services/operator_settings/testdata/baseline/operator-config-input-index.json"

// InputInventory indexes deterministic operator-config inputs and expected loader outcomes.
type InputInventory struct {
	FormatVersion      string      `json:"formatVersion"`
	UnknownFieldPolicy string      `json:"unknownFieldPolicy"`
	PrecedenceChain    string      `json:"precedenceChain"`
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

	ExpectedConfig    *ConfigExpectation   `json:"expectedConfig,omitempty"`
	ResolveLayers     *ResolveLayers       `json:"resolveLayers,omitempty"`
	PrecedenceWinners *PrecedenceWinners   `json:"precedenceWinners,omitempty"`
	ExpectedResolved  *ResolvedExpectation `json:"expectedResolved,omitempty"`
	ErrorFragments    []string             `json:"errorFragments,omitempty"`
}

// ConfigExpectation records expected generated-document decode/load outputs.
type ConfigExpectation struct {
	BackendScopeID string           `json:"backendScopeID,omitempty"`
	Defaults       DefaultsSnapshot `json:"defaults"`
	WorkerPresets  []WorkerPreset   `json:"workerPresets,omitempty"`
}

// DefaultsSnapshot is the trimmed defaults shape asserted by inventory table tests.
type DefaultsSnapshot struct {
	WorkerModelProvider string `json:"workerModelProvider,omitempty"`
	WorkerModel         string `json:"workerModel,omitempty"`
}

// ResolveLayers supplies file/env/flag inputs for Resolve inventory cases.
type ResolveLayers struct {
	FileFixture  string            `json:"fileFixture,omitempty"`
	FileDefaults DefaultsSnapshot  `json:"fileDefaults,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	Flag         FlagSnapshot      `json:"flag,omitempty"`
}

// FlagSnapshot records CLI flag overrides for Resolve inventory cases.
type FlagSnapshot struct {
	WorkerModelProvider string `json:"workerModelProvider,omitempty"`
	WorkerModel         string `json:"workerModel,omitempty"`
}

// PrecedenceWinners records which layer won independently per defaults field.
type PrecedenceWinners struct {
	WorkerModelProviderSource string `json:"workerModelProviderSource,omitempty"`
	WorkerModelSource         string `json:"workerModelSource,omitempty"`
}

// ResolvedExpectation records expected Resolve outputs for accepted resolve cases.
type ResolvedExpectation struct {
	WorkerModelProvider string `json:"workerModelProvider,omitempty"`
	WorkerModel         string `json:"workerModel,omitempty"`
}

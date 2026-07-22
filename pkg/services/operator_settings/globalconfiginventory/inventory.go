// Package globalconfiginventory documents the shared ~/.you-agent-factory/config.json
// field topology and ownership of the Operator Settings service without changing
// loader behavior.
package globalconfiginventory

// FormatVersion identifies the global config topology inventory document shape.
const FormatVersion = "global-config-topology/v1"

const (
	ownerOperatorSettings = "operator_settings"
	ownerNone             = "none"

	strictnessOperatorSettingsStrictDecode = "operator_settings strict decode (DisallowUnknownFields on parse)"
	strictnessOperatorSettingsIdentityLoad = "operator_settings identity tolerant load (unknown top-level keys ignored on read; preserved on persist)"
	strictnessFileOnly                     = "file-only (no env or flag layers)"
)

// Inventory is the canonical global config topology document.
type Inventory struct {
	FormatVersion      string               `json:"formatVersion"`
	SharedConfigFile   SharedConfigFile     `json:"sharedConfigFile"`
	SharedFileSplit    SharedFileSplit      `json:"sharedFileSplit"`
	PrecedenceChain    string               `json:"precedenceChain"`
	UnknownFieldPolicy []UnknownFieldPolicy `json:"unknownFieldPolicy"`
	Fields             []FieldRecord        `json:"fields"`
}

// SharedConfigFile records the shared on-disk config location.
type SharedConfigFile struct {
	RelativePath string `json:"relativePath"`
	ResolvedBy   string `json:"resolvedBy"`
}

// SharedFileSplit documents ownership within the shared JSON file.
type SharedFileSplit struct {
	Summary string      `json:"summary"`
	Owners  []FileOwner `json:"owners"`
}

// FileOwner records one package's ownership within the shared config file.
type FileOwner struct {
	Package    string   `json:"package"`
	Owns       []string `json:"owns"`
	Tolerates  []string `json:"tolerates,omitempty"`
	DoesNotOwn []string `json:"doesNotOwn,omitempty"`
}

// UnknownFieldPolicy states loader-specific unknown-field handling.
type UnknownFieldPolicy struct {
	Package string `json:"package"`
	Policy  string `json:"policy"`
}

// FieldRecord inventories one accepted global config field.
type FieldRecord struct {
	ID                   string   `json:"id"`
	JSONPath             string   `json:"jsonPath"`
	JSONName             string   `json:"jsonName"`
	ValueType            string   `json:"valueType"`
	ParentField          string   `json:"parentField,omitempty"`
	DefaultEmptyBehavior string   `json:"defaultEmptyBehavior"`
	Strictness           string   `json:"strictness"`
	PersistenceOwner     string   `json:"persistenceOwner"`
	ParseOwner           string   `json:"parseOwner"`
	PrecedenceLayers     []string `json:"precedenceLayers,omitempty"`
	EnvironmentVariable  string   `json:"environmentVariable,omitempty"`
	FlagName             string   `json:"flagName,omitempty"`
	Notes                string   `json:"notes,omitempty"`
}

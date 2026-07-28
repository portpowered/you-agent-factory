// Package mockworkers loads and validates the mock-workers JSON contract. The
// inventory types in this file document current production-loader acceptance
// without changing parse or validation behavior.
package mockworkers

// FormatVersion identifies the mock-worker topology inventory document shape.
const FormatVersion = "mock-workers-topology/v1"

const (
	ownerDecode   = "decode"
	ownerValidate = "validate"
)

// Inventory is the canonical mock-worker authoring topology document.
type Inventory struct {
	FormatVersion             string                    `json:"formatVersion"`
	LoaderEntrypoints         []string                  `json:"loaderEntrypoints"`
	UnknownFieldPolicy        string                    `json:"unknownFieldPolicy"`
	EntrySelectionPolicy      string                    `json:"entrySelectionPolicy"`
	RunTypeUnion              RunTypeUnion              `json:"runTypeUnion"`
	UnmatchedDispatchPolicies []UnmatchedDispatchPolicy `json:"unmatchedDispatchPolicies"`
	ValidationBoundaries      []ValidationBoundary      `json:"validationBoundaries"`
	NotAcceptedCapabilities   []NotAcceptedCapability   `json:"notAcceptedCapabilities"`
	Fields                    []FieldRecord             `json:"fields"`
}

// RunTypeUnion documents accepted runType values and nested config requirements.
type RunTypeUnion struct {
	Summary string          `json:"summary"`
	Values  []RunTypeRecord `json:"values"`
}

// RunTypeRecord inventories one accepted runType variant.
type RunTypeRecord struct {
	Value              string `json:"value"`
	NestedConfig       string `json:"nestedConfig,omitempty"`
	NestedConfigPolicy string `json:"nestedConfigPolicy,omitempty"`
}

// UnmatchedDispatchPolicy inventories one accepted unmatched-dispatch value.
type UnmatchedDispatchPolicy struct {
	Value           string `json:"value"`
	OmittedBehavior bool   `json:"omittedBehavior,omitempty"`
	Summary         string `json:"summary"`
}

// ValidationBoundary records a strict rejection owned by decode or Validate.
type ValidationBoundary struct {
	Condition    string `json:"condition"`
	Owner        string `json:"owner"`
	ErrorPattern string `json:"errorPattern"`
}

// NotAcceptedCapability documents batch-language categories the loader does not accept.
type NotAcceptedCapability struct {
	Category string `json:"category"`
	Reason   string `json:"reason"`
}

// FieldRecord inventories one accepted mock-worker JSON field.
type FieldRecord struct {
	ID                   string `json:"id"`
	JSONPath             string `json:"jsonPath"`
	JSONName             string `json:"jsonName"`
	ValueType            string `json:"valueType"`
	ParentField          string `json:"parentField,omitempty"`
	Required             string `json:"required"`
	DefaultEmptyBehavior string `json:"defaultEmptyBehavior"`
	ValidationOwner      string `json:"validationOwner"`
	Notes                string `json:"notes,omitempty"`
}

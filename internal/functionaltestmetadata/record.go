package functionaltestmetadata

// Classification labels whether an inventoried Test* is a customer scenario or
// harness/internal verification. Customer scenario counts use only
// ClassificationCustomer records.
type Classification string

const (
	// ClassificationCustomer is a customer-facing functional scenario.
	ClassificationCustomer Classification = "customer"
	// ClassificationHarness is harness/internal or helper-only verification.
	// These records remain in the inventory for later report rendering but do
	// not increment customer scenario counts.
	ClassificationHarness Classification = "harness"
)

// Record is one inventoried top-level Test* declaration.
type Record struct {
	// File is the repository-relative source path using forward slashes.
	File string `json:"file"`
	// Package is the Go package name declared in the source file.
	Package string `json:"package"`
	// Name is the Test* function name.
	Name string `json:"name"`
	// Line is the 1-based source line of the function declaration.
	Line int `json:"line"`
	// Description is the first sentence of the conventional Go doc comment.
	// Empty when Undocumented is true.
	Description string `json:"description,omitempty"`
	// Undocumented is true when the declaration has no conventional Go doc
	// comment first sentence.
	Undocumented bool `json:"undocumented"`
	// BuildTags are the file-level //go:build (or legacy // +build) constraint
	// expressions that apply to this test. Empty when the file has no build
	// constraints; never fabricated with a default tag.
	BuildTags []string `json:"buildTags,omitempty"`
	// Golden is an explicit fixture/manifest path declared for this test via a
	// //golden: comment or a test-owned golden string declaration. Empty when
	// no golden is declared; never fabricated.
	Golden string `json:"golden,omitempty"`
	// Classification is customer versus harness/helper verification.
	Classification Classification `json:"classification"`
}

// Identity returns the stable catalog identity for this record.
func (r Record) Identity() string {
	return r.File + "::" + r.Name
}

// IsCustomerScenario reports whether this record counts toward customer
// scenario totals.
func (r Record) IsCustomerScenario() bool {
	return r.Classification == ClassificationCustomer
}

// CustomerScenarioCount returns the number of inventoried top-level customer
// Test* records after excluding harness/internal and helper-only entries.
func CustomerScenarioCount(records []Record) int {
	count := 0
	for _, record := range records {
		if record.IsCustomerScenario() {
			count++
		}
	}
	return count
}

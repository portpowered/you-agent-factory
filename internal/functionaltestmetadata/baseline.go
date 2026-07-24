package functionaltestmetadata

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

const (
	// BaselineStage identifies the functional undocumented-test ledger.
	BaselineStage = "functional-undocumented-tests"
	// BaselineVersion is the current on-disk baseline schema version.
	BaselineVersion = 1
)

// BaselineEntry is one exact undocumented customer Test* identity.
type BaselineEntry struct {
	File string `json:"file"`
	Name string `json:"name"`
}

// Identity returns the stable catalog identity for this baseline entry.
func (e BaselineEntry) Identity() string {
	return e.File + "::" + e.Name
}

// Baseline is an exact deletion-only ledger of undocumented customer Test*
// identities. Removals are allowed; expansions are rejected.
type Baseline struct {
	Version int             `json:"version"`
	Stage   string          `json:"stage"`
	Entries []BaselineEntry `json:"entries"`
}

// UndocumentedCustomerIdentities returns stable identities for customer
// scenario records that lack a conventional Go-doc description. Harness and
// internal helpers are excluded.
func UndocumentedCustomerIdentities(records []Record) []string {
	identities := make([]string, 0)
	for _, record := range records {
		if record.Undocumented && record.IsCustomerScenario() {
			identities = append(identities, record.Identity())
		}
	}
	slices.Sort(identities)
	return slices.Clip(slices.Compact(identities))
}

// BaselineFromRecords builds an exact baseline from inventoried records,
// including only undocumented customer scenarios.
func BaselineFromRecords(records []Record) Baseline {
	identities := UndocumentedCustomerIdentities(records)
	entries := make([]BaselineEntry, 0, len(identities))
	for _, identity := range identities {
		file, name, ok := splitIdentity(identity)
		if !ok {
			continue
		}
		entries = append(entries, BaselineEntry{File: file, Name: name})
	}
	return Baseline{
		Version: BaselineVersion,
		Stage:   BaselineStage,
		Entries: entries,
	}
}

// LoadBaseline reads an exact undocumented-test baseline JSON file.
func LoadBaseline(path string) (Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Baseline{}, fmt.Errorf("read undocumented-test baseline %s: %w", normalizePath(path), err)
	}
	baseline, err := ParseBaseline(data)
	if err != nil {
		return Baseline{}, fmt.Errorf("parse undocumented-test baseline %s: %w", normalizePath(path), err)
	}
	return baseline, nil
}

// ParseBaseline decodes and normalizes an exact undocumented-test baseline.
func ParseBaseline(data []byte) (Baseline, error) {
	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return Baseline{}, err
	}
	if baseline.Version == 0 {
		baseline.Version = BaselineVersion
	}
	if baseline.Stage == "" {
		baseline.Stage = BaselineStage
	}
	for i := range baseline.Entries {
		baseline.Entries[i].File = normalizePath(baseline.Entries[i].File)
		baseline.Entries[i].Name = strings.TrimSpace(baseline.Entries[i].Name)
		if baseline.Entries[i].File == "" || baseline.Entries[i].Name == "" {
			return Baseline{}, fmt.Errorf("baseline entry %d is missing file or name", i)
		}
	}
	slices.SortFunc(baseline.Entries, compareBaselineEntries)
	baseline.Entries = slices.CompactFunc(baseline.Entries, func(a, b BaselineEntry) bool {
		return a.Identity() == b.Identity()
	})
	return baseline, nil
}

// Identities returns sorted unique baseline identities.
func (b Baseline) Identities() []string {
	identities := make([]string, 0, len(b.Entries))
	for _, entry := range b.Entries {
		identities = append(identities, entry.Identity())
	}
	slices.Sort(identities)
	return slices.Clip(slices.Compact(identities))
}

// CheckAgainstBaseline compares inventoried undocumented customer tests with
// an exact deletion-only baseline. Success requires the undocumented set to be
// identical to or a strict subset of the baseline. Newly undocumented customer
// tests absent from the baseline fail with an actionable identity.
func CheckAgainstBaseline(records []Record, baseline Baseline) error {
	allowed := make(map[string]struct{}, len(baseline.Entries))
	for _, entry := range baseline.Entries {
		allowed[entry.Identity()] = struct{}{}
	}

	var unexpected []string
	for _, identity := range UndocumentedCustomerIdentities(records) {
		if _, ok := allowed[identity]; !ok {
			unexpected = append(unexpected, identity)
		}
	}
	if len(unexpected) == 0 {
		return nil
	}
	slices.Sort(unexpected)
	return fmt.Errorf(
		"new undocumented customer test(s) absent from deletion-only baseline: %s",
		strings.Join(unexpected, ", "),
	)
}

// ValidateBaselineUpdate enforces deletion-only baseline edits: the proposed
// baseline may only remove identities. Expanding the baseline with new
// undocumented identities is rejected.
func ValidateBaselineUpdate(previous, next Baseline) error {
	allowed := make(map[string]struct{}, len(previous.Entries))
	for _, entry := range previous.Entries {
		allowed[entry.Identity()] = struct{}{}
	}

	var expansions []string
	for _, identity := range next.Identities() {
		if _, ok := allowed[identity]; !ok {
			expansions = append(expansions, identity)
		}
	}
	if len(expansions) == 0 {
		return nil
	}
	slices.Sort(expansions)
	return fmt.Errorf(
		"illegal undocumented-test baseline expansion (deletion-only): %s",
		strings.Join(expansions, ", "),
	)
}

func compareBaselineEntries(a, b BaselineEntry) int {
	if a.File != b.File {
		return strings.Compare(a.File, b.File)
	}
	return strings.Compare(a.Name, b.Name)
}

func splitIdentity(identity string) (file, name string, ok bool) {
	file, name, found := strings.Cut(identity, "::")
	if !found || file == "" || name == "" {
		return "", "", false
	}
	return file, name, true
}

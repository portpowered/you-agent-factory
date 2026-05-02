package workers

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ProviderErrorCorpusEntry is one shared raw provider-failure fixture used by
// worker unit tests and functional smoke coverage.
type ProviderErrorCorpusEntry struct {
	Name                  string                         `json:"name"`
	Provider              ModelProvider                  `json:"provider"`
	RawProviderFamily     string                         `json:"raw_provider_family"`
	Category              string                         `json:"category"`
	UpstreamSourceCase    string                         `json:"upstream_source_case"`
	ExitCode              int                            `json:"exit_code"`
	Stdout                string                         `json:"stdout"`
	Stderr                string                         `json:"stderr"`
	ExpectedType          interfaces.ProviderErrorType   `json:"expected_type"`
	ExpectedFamily        interfaces.ProviderErrorFamily `json:"expected_family"`
	Retryable             bool                           `json:"retryable"`
	TriggersThrottlePause bool                           `json:"triggers_throttle_pause"`
	Supported             bool                           `json:"supported"`
	Notes                 string                         `json:"notes,omitempty"`
}

// CommandResult renders the raw shared fixture into the provider subprocess
// contract used by normalization tests and smoke harnesses.
func (e ProviderErrorCorpusEntry) CommandResult() CommandResult {
	return CommandResult{
		ExitCode: e.ExitCode,
		Stdout:   []byte(e.Stdout),
		Stderr:   []byte(e.Stderr),
	}
}

// RepeatedCommandResults expands one shared failure shape into a fixed number
// of repeated provider command results for bounded retry and throttle tests.
func (e ProviderErrorCorpusEntry) RepeatedCommandResults(count int) []CommandResult {
	results := make([]CommandResult, 0, count)
	for range count {
		results = append(results, e.CommandResult())
	}
	return results
}

type providerErrorCorpusFile struct {
	Entries []ProviderErrorCorpusEntry `json:"entries"`
}

// ProviderErrorCorpus is the cached shared provider-failure fixture set.
type ProviderErrorCorpus struct {
	entriesByName map[string]ProviderErrorCorpusEntry
	allEntries    []ProviderErrorCorpusEntry
}

// Entry returns the named shared fixture.
func (c ProviderErrorCorpus) Entry(name string) (ProviderErrorCorpusEntry, bool) {
	entry, ok := c.entriesByName[name]
	return entry, ok
}

// SupportedEntriesForCategory returns the currently supported fixtures for one
// normalized provider-failure category.
func (c ProviderErrorCorpus) SupportedEntriesForCategory(category string) []ProviderErrorCorpusEntry {
	entries := make([]ProviderErrorCorpusEntry, 0, len(c.allEntries))
	for _, entry := range c.allEntries {
		if entry.Supported && entry.Category == category {
			entries = append(entries, entry)
		}
	}
	return entries
}

//go:embed testdata/provider_error_corpus.json
var providerErrorCorpusJSON []byte

var (
	providerErrorCorpusOnce sync.Once
	providerErrorCorpus     ProviderErrorCorpus
	providerErrorCorpusErr  error
)

// LoadProviderErrorCorpus returns the shared provider-failure fixture corpus.
func LoadProviderErrorCorpus() (ProviderErrorCorpus, error) {
	providerErrorCorpusOnce.Do(func() {
		providerErrorCorpus, providerErrorCorpusErr = loadProviderErrorCorpus()
	})
	return providerErrorCorpus, providerErrorCorpusErr
}

func loadProviderErrorCorpus() (ProviderErrorCorpus, error) {
	var raw providerErrorCorpusFile
	if err := json.Unmarshal(providerErrorCorpusJSON, &raw); err != nil {
		return ProviderErrorCorpus{}, fmt.Errorf("decode provider error corpus: %w", err)
	}
	if len(raw.Entries) == 0 {
		return ProviderErrorCorpus{}, fmt.Errorf("decode provider error corpus: no entries")
	}

	entriesByName := make(map[string]ProviderErrorCorpusEntry, len(raw.Entries))
	for _, entry := range raw.Entries {
		if err := validateProviderErrorCorpusEntry(entry); err != nil {
			return ProviderErrorCorpus{}, err
		}
		if _, exists := entriesByName[entry.Name]; exists {
			return ProviderErrorCorpus{}, fmt.Errorf("decode provider error corpus: duplicate entry %q", entry.Name)
		}
		entriesByName[entry.Name] = entry
	}

	return ProviderErrorCorpus{
		entriesByName: entriesByName,
		allEntries:    append([]ProviderErrorCorpusEntry(nil), raw.Entries...),
	}, nil
}

func validateProviderErrorCorpusEntry(entry ProviderErrorCorpusEntry) error {
	if entry.Name == "" {
		return fmt.Errorf("decode provider error corpus: missing entry name")
	}
	if entry.Provider == "" {
		return fmt.Errorf("decode provider error corpus: entry %q missing provider", entry.Name)
	}
	if entry.RawProviderFamily == "" {
		return fmt.Errorf("decode provider error corpus: entry %q missing raw provider family", entry.Name)
	}
	if entry.Category == "" {
		return fmt.Errorf("decode provider error corpus: entry %q missing category", entry.Name)
	}
	if entry.UpstreamSourceCase == "" {
		return fmt.Errorf("decode provider error corpus: entry %q missing upstream source case", entry.Name)
	}
	if entry.ExpectedType == "" {
		return fmt.Errorf("decode provider error corpus: entry %q missing expected type", entry.Name)
	}
	if entry.ExpectedFamily == "" {
		return fmt.Errorf("decode provider error corpus: entry %q missing expected family", entry.Name)
	}
	if entry.ExpectedFamily == interfaces.ProviderErrorFamilyThrottle && !entry.TriggersThrottlePause {
		return fmt.Errorf("decode provider error corpus: entry %q throttle family must trigger throttle pause", entry.Name)
	}
	if entry.TriggersThrottlePause && !entry.Retryable {
		return fmt.Errorf("decode provider error corpus: entry %q throttle pause requires retryable=true", entry.Name)
	}
	return nil
}

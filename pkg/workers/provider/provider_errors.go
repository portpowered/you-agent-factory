package provider

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

// ProviderError is the shared normalized provider failure contract. Provider
// implementations should return this typed error so executor, pause, and
// customer-messaging logic can make deterministic decisions without parsing raw
// provider output at every call site.
type ProviderError struct {
	Family          workerexecution.WorkFailureFamily
	Type            workerexecution.WorkFailureType
	Message         string
	ProviderSession *workerexecution.ProviderSessionMetadata
	Diagnostics     *workerexecution.WorkDiagnostics
	Cause           error
}

// ProviderFailureResult is the pure output of provider failure parsing. It
// deliberately carries only the canonical reason and customer-visible message;
// runtime policy is derived from Reason when the result crosses into execution.
type ProviderFailureResult struct {
	Reason  workerexecution.WorkFailureType
	Message string
}

func ProviderFailureInternalCauseError(cause string) error {
	cause = strings.TrimSpace(cause)
	if cause == "" {
		return nil
	}
	return errors.New(cause)
}

func NewProviderError(errorType workerexecution.WorkFailureType, message string, cause error) *ProviderError {
	return NewProviderErrorFromResult(ProviderFailureResult{
		Reason:  errorType,
		Message: message,
	}, cause)
}

// NewProviderErrorFromResult turns a pure parse result into the normalized
// execution error while deriving all runtime policy from its canonical reason.
func NewProviderErrorFromResult(result ProviderFailureResult, cause error) *ProviderError {
	return &ProviderError{
		Family:  providerFailurePolicyForReason(result.Reason).Family,
		Type:    result.Reason,
		Message: result.Message,
		Cause:   cause,
	}
}

func newProviderErrorFromResultWithDiagnostics(result ProviderFailureResult, cause error, session *workerexecution.ProviderSessionMetadata, diagnostics *workerexecution.WorkDiagnostics) *ProviderError {
	err := NewProviderErrorFromResult(result, cause)
	err.ProviderSession = workerexecution.CloneProviderSessionMetadata(session)
	err.Diagnostics = workerexecution.CloneWorkDiagnostics(diagnostics)
	return err
}

func NewProviderErrorWithSession(errorType workerexecution.WorkFailureType, message string, cause error, session *workerexecution.ProviderSessionMetadata) *ProviderError {
	err := NewProviderError(errorType, message, cause)
	err.ProviderSession = workerexecution.CloneProviderSessionMetadata(session)
	return err
}

func newProviderErrorWithDiagnostics(errorType workerexecution.WorkFailureType, message string, cause error, session *workerexecution.ProviderSessionMetadata, diagnostics *workerexecution.WorkDiagnostics) *ProviderError {
	return newProviderErrorFromResultWithDiagnostics(ProviderFailureResult{
		Reason:  errorType,
		Message: message,
	}, cause, session, diagnostics)
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider error: %s", e.Type)
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func ClassifyProviderFailure(err *ProviderError) workerexecution.WorkFailureDecision {
	if err == nil {
		return workerexecution.WorkFailureDecision{}
	}
	return providerFailurePolicyForReason(err.Type).Decision
}

// WorkFailureMetadataFromError projects a provider-shaped execution error onto
// the in-process failure contract carried on WorkResult.FailureMetadata.
func WorkFailureMetadataFromError(err *ProviderError) *workerexecution.WorkFailureMetadata {
	if err == nil {
		return nil
	}
	return &workerexecution.WorkFailureMetadata{
		Family: providerFailurePolicyForReason(err.Type).Family,
		Type:   err.Type,
	}
}

// NormalizeProviderExecutionError projects raw execution failures that affect
// retry policy onto the shared provider failure contract before retry decisions
// are made.
func NormalizeProviderExecutionError(err error) *ProviderError {
	if err == nil {
		return nil
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewProviderError(workerexecution.WorkFailureTypeTimeout, "execution timeout", err)
	}
	return nil
}

// ProviderErrorCorpusEntry is one shared raw provider-failure fixture used by
// worker unit tests and functional smoke coverage.
type ProviderErrorCorpusEntry struct {
	Name                  string                            `json:"name"`
	Provider              modelprovider.ID                  `json:"provider"`
	RawProviderFamily     string                            `json:"raw_provider_family"`
	Category              string                            `json:"category"`
	UpstreamSourceCase    string                            `json:"upstream_source_case"`
	ExitCode              int                               `json:"exit_code"`
	Stdout                string                            `json:"stdout"`
	Stderr                string                            `json:"stderr"`
	ExpectedType          workerexecution.WorkFailureType   `json:"expected_type"`
	ExpectedFamily        workerexecution.WorkFailureFamily `json:"expected_family"`
	ExpectedMessage       string                            `json:"expected_message,omitempty"`
	Retryable             bool                              `json:"retryable"`
	TriggersThrottlePause bool                              `json:"triggers_throttle_pause"`
	Supported             bool                              `json:"supported"`
	RejectMessageContains []string                          `json:"reject_message_contains,omitempty"`
	Notes                 string                            `json:"notes,omitempty"`
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

// Entries returns all corpus fixtures in stable order.
func (c ProviderErrorCorpus) Entries() []ProviderErrorCorpusEntry {
	return append([]ProviderErrorCorpusEntry(nil), c.allEntries...)
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
	if entry.Provider == modelprovider.Claude && entry.ExpectedMessage == "" {
		return fmt.Errorf("decode provider error corpus: Claude entry %q missing expected message", entry.Name)
	}
	if entry.ExpectedFamily == workerexecution.WorkFailureFamilyThrottle && !entry.TriggersThrottlePause {
		return fmt.Errorf("decode provider error corpus: entry %q throttle family must trigger throttle pause", entry.Name)
	}
	if entry.TriggersThrottlePause && !entry.Retryable {
		return fmt.Errorf("decode provider error corpus: entry %q throttle pause requires retryable=true", entry.Name)
	}
	return nil
}

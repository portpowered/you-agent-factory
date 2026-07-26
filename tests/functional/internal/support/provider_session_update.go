package support

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ProviderSessionUpdateFunctionalGoldensEnv gates expected golden rewrites.
// Normal CI must leave this unset (or any value other than "1") so drift fails
// without rewriting fixture files.
const ProviderSessionUpdateFunctionalGoldensEnv = "UPDATE_FUNCTIONAL_GOLDENS"

// ProviderSessionFunctionalGoldensUpdateEnabled reports whether expected golden
// rewrites are authorized for this process.
func ProviderSessionFunctionalGoldensUpdateEnabled() bool {
	return os.Getenv(ProviderSessionUpdateFunctionalGoldensEnv) == "1"
}

// ProviderSessionGoldensUpdatedError reports that expected golden files were
// rewritten because UPDATE_FUNCTIONAL_GOLDENS=1 authorized an update. Callers
// should fail the current run and re-run without the env set to verify the
// new goldens.
type ProviderSessionGoldensUpdatedError struct {
	CaseID  string
	Paths   []string
	Message string
}

func (e *ProviderSessionGoldensUpdatedError) Error() string {
	caseID := e.CaseID
	if caseID == "" {
		caseID = "(unknown)"
	}
	msg := e.Message
	if msg == "" {
		msg = "expected golden files were rewritten; re-run without UPDATE_FUNCTIONAL_GOLDENS=1"
	}
	if len(e.Paths) == 0 {
		return fmt.Sprintf("provider-session golden update case %q: %s", caseID, msg)
	}
	return fmt.Sprintf(
		"provider-session golden update case %q: %s (rewrote %s)",
		caseID,
		msg,
		strings.Join(e.Paths, ", "),
	)
}

// CompareOrUpdateProviderSessionGoldens structurally compares observed public
// metadata to the loaded expected goldens. On drift:
//   - without UPDATE_FUNCTIONAL_GOLDENS=1, returns the compare error and does
//     not rewrite any files;
//   - with UPDATE_FUNCTIONAL_GOLDENS=1, rewrites the three expected golden files
//     from the observed values and returns ProviderSessionGoldensUpdatedError.
//
// Missing fixtures are never treated as a pass: callers must load a complete
// case first, and load/resolve already fail loudly for absent files.
func CompareOrUpdateProviderSessionGoldens(
	loaded ProviderSessionCase,
	observed ProviderSessionObservedGoldens,
) error {
	err := CompareProviderSessionGoldens(loaded.Manifest, loaded.Expected, observed)
	if err == nil {
		return nil
	}

	var compareErr *ProviderSessionCompareError
	if !errors.As(err, &compareErr) {
		return err
	}
	if !ProviderSessionFunctionalGoldensUpdateEnabled() {
		return err
	}

	rewritten, writeErr := WriteProviderSessionExpectedGoldens(loaded.Paths, observed)
	if writeErr != nil {
		return writeErr
	}
	return &ProviderSessionGoldensUpdatedError{
		CaseID:  strings.TrimSpace(loaded.Manifest.ID),
		Paths:   rewritten,
		Message: "expected golden files were rewritten; re-run without UPDATE_FUNCTIONAL_GOLDENS=1",
	}
}

// WriteProviderSessionExpectedGoldens rewrites the three expected public
// metadata golden files from observed values. Intended for the explicit
// UPDATE_FUNCTIONAL_GOLDENS=1 update path only.
func WriteProviderSessionExpectedGoldens(
	paths ProviderSessionManifestPaths,
	observed ProviderSessionObservedGoldens,
) ([]string, error) {
	sessionBytes, err := formatProviderSessionJSONDocument(observed.ProviderSession)
	if err != nil {
		return nil, fmt.Errorf("format expected-provider-session for rewrite: %w", err)
	}
	eventsBytes, err := formatProviderSessionNDJSONDocument(observed.ResponseEvents)
	if err != nil {
		return nil, fmt.Errorf("format expected-response-events for rewrite: %w", err)
	}
	resultBytes, err := formatProviderSessionJSONDocument(observed.InvocationResult)
	if err != nil {
		return nil, fmt.Errorf("format expected-invocation-result for rewrite: %w", err)
	}

	writes := []struct {
		role string
		path string
		data []byte
	}{
		{role: "expected-provider-session", path: paths.ExpectedProviderSession, data: sessionBytes},
		{role: "expected-response-events", path: paths.ExpectedResponseEvents, data: eventsBytes},
		{role: "expected-invocation-result", path: paths.ExpectedInvocationResult, data: resultBytes},
	}
	rewritten := make([]string, 0, len(writes))
	for _, write := range writes {
		if strings.TrimSpace(write.path) == "" {
			return rewritten, fmt.Errorf("provider-session golden rewrite role %q: path is empty", write.role)
		}
		if err := os.WriteFile(write.path, write.data, 0o644); err != nil {
			return rewritten, fmt.Errorf("provider-session golden rewrite role %q path %s: %w", write.role, write.path, err)
		}
		rewritten = append(rewritten, write.path)
	}
	return rewritten, nil
}

func formatProviderSessionJSONDocument(raw json.RawMessage) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("JSON document is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func formatProviderSessionNDJSONDocument(records []json.RawMessage) ([]byte, error) {
	if len(records) == 0 {
		return []byte{}, nil
	}
	var builder strings.Builder
	for i, record := range records {
		trimmed := bytes.TrimSpace(record)
		if len(trimmed) == 0 {
			return nil, fmt.Errorf("NDJSON record %d is empty", i)
		}
		decoder := json.NewDecoder(bytes.NewReader(trimmed))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("NDJSON record %d: %w", i, err)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("NDJSON record %d: %w", i, err)
		}
		builder.Write(encoded)
		builder.WriteByte('\n')
	}
	return []byte(builder.String()), nil
}

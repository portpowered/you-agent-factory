package providertestdata

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

//go:embed provider_error_corpus.json
var corpusJSON []byte

// Entry is one shared provider-failure fixture entry for owned-package tests.
type Entry struct {
	Name                  string                          `json:"name"`
	Provider              modelprovider.Provider          `json:"provider"`
	ExitCode              int                             `json:"exit_code"`
	Stdout                string                          `json:"stdout"`
	Stderr                string                          `json:"stderr"`
	ExpectedType          workerexecution.WorkFailureType `json:"expected_type"`
	ExpectedMessage       string                          `json:"expected_message,omitempty"`
	RejectMessageContains []string                        `json:"reject_message_contains,omitempty"`
}

// FailureInput is the subprocess output contract used by owned failure parsers.
type FailureInput struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// FailureInput renders the corpus entry into parser input.
func (e Entry) FailureInput() FailureInput {
	return FailureInput{
		ExitCode: e.ExitCode,
		Stdout:   []byte(e.Stdout),
		Stderr:   []byte(e.Stderr),
	}
}

var entriesByName map[string]Entry

func init() {
	var raw struct {
		Entries []Entry `json:"entries"`
	}
	if err := json.Unmarshal(corpusJSON, &raw); err != nil {
		panic(fmt.Sprintf("providertestdata: decode corpus: %v", err))
	}
	entriesByName = make(map[string]Entry, len(raw.Entries))
	for _, entry := range raw.Entries {
		entriesByName[entry.Name] = entry
	}
}

// MustEntry returns one corpus entry by name or fails the test.
func MustEntry(t *testing.T, name string) Entry {
	t.Helper()
	entry, ok := entriesByName[name]
	if !ok {
		t.Fatalf("providertestdata entry %q not found", name)
	}
	return entry
}

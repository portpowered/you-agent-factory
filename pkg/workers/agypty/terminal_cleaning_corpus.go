package agypty

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
)

// TerminalCleaningCase is one hermetic fixture for CleanTerminal.
type TerminalCleaningCase struct {
	Name  string `json:"name"`
	Raw   string `json:"raw_b64"`
	Want  string `json:"want"`
	Empty bool   `json:"empty,omitempty"`
}

type terminalCleaningCasesFile struct {
	Cases []TerminalCleaningCase `json:"cases"`
}

// TerminalCleaningCorpus is the cached terminal-cleaning regression fixture set.
type TerminalCleaningCorpus struct {
	casesByName map[string]TerminalCleaningCase
	allCases    []TerminalCleaningCase
}

// Case returns the named fixture.
func (c TerminalCleaningCorpus) Case(name string) (TerminalCleaningCase, bool) {
	entry, ok := c.casesByName[name]
	return entry, ok
}

// Cases returns all fixtures in file order.
func (c TerminalCleaningCorpus) Cases() []TerminalCleaningCase {
	return append([]TerminalCleaningCase(nil), c.allCases...)
}

//go:embed testdata/terminal_cleaning_corpus.json
var terminalCleaningCasesJSON []byte

var (
	terminalCleaningCorpusOnce sync.Once
	terminalCleaningCorpus     TerminalCleaningCorpus
	terminalCleaningCorpusErr  error
)

// LoadTerminalCleaningCorpus returns the shared terminal-cleaning fixture corpus.
func LoadTerminalCleaningCorpus() (TerminalCleaningCorpus, error) {
	terminalCleaningCorpusOnce.Do(func() {
		terminalCleaningCorpus, terminalCleaningCorpusErr = loadTerminalCleaningCorpus()
	})
	return terminalCleaningCorpus, terminalCleaningCorpusErr
}

func loadTerminalCleaningCorpus() (TerminalCleaningCorpus, error) {
	var raw terminalCleaningCasesFile
	if err := json.Unmarshal(terminalCleaningCasesJSON, &raw); err != nil {
		return TerminalCleaningCorpus{}, fmt.Errorf("decode terminal cleaning cases: %w", err)
	}
	if len(raw.Cases) == 0 {
		return TerminalCleaningCorpus{}, fmt.Errorf("decode terminal cleaning cases: no cases")
	}

	casesByName := make(map[string]TerminalCleaningCase, len(raw.Cases))
	for _, entry := range raw.Cases {
		if entry.Name == "" {
			return TerminalCleaningCorpus{}, fmt.Errorf("decode terminal cleaning cases: missing name")
		}
		if _, exists := casesByName[entry.Name]; exists {
			return TerminalCleaningCorpus{}, fmt.Errorf("decode terminal cleaning cases: duplicate name %q", entry.Name)
		}
		casesByName[entry.Name] = entry
	}
	return TerminalCleaningCorpus{
		casesByName: casesByName,
		allCases:    raw.Cases,
	}, nil
}

// RawBytes decodes the fixture capture bytes.
func (c TerminalCleaningCase) RawBytes() ([]byte, error) {
	if c.Raw == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(c.Raw)
	if err != nil {
		return nil, fmt.Errorf("decode raw_b64 for %q: %w", c.Name, err)
	}
	return raw, nil
}

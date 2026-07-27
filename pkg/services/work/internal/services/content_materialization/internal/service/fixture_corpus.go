package service

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

// URLMaterializationCase is one regression fixture for MaterializeContentURL.
type URLMaterializationCase struct {
	Name         string                            `json:"name"`
	Setup        string                            `json:"setup,omitempty"`
	URL          string                            `json:"url,omitempty"`
	FileContent  string                            `json:"fileContent,omitempty"`
	ResponseBody string                            `json:"responseBody,omitempty"`
	ContentType  string                            `json:"contentType,omitempty"`
	StatusCode   int                               `json:"statusCode,omitempty"`
	MaxBytes     int64                             `json:"maxBytes,omitempty"`
	Expect       URLMaterializationCaseExpectation `json:"expect"`
}

// URLMaterializationCaseExpectation describes the expected materialization outcome.
type URLMaterializationCaseExpectation struct {
	Outcome              string   `json:"outcome"`
	ErrorIs              string   `json:"errorIs,omitempty"`
	ErrorContains        []string `json:"errorContains,omitempty"`
	SameAsLocalPath      bool     `json:"sameAsLocalPath,omitempty"`
	BodyMatch            bool     `json:"bodyMatch,omitempty"`
	TempPrefix           string   `json:"tempPrefix,omitempty"`
	TempRemovedOnCleanup bool     `json:"tempRemovedOnCleanup,omitempty"`
}

type urlMaterializationCasesFile struct {
	Cases []URLMaterializationCase `json:"cases"`
}

// URLMaterializationCorpus is the cached materialization regression fixture set.
type URLMaterializationCorpus struct {
	casesByName map[string]URLMaterializationCase
	allCases    []URLMaterializationCase
}

// Case returns the named fixture.
func (c URLMaterializationCorpus) Case(name string) (URLMaterializationCase, bool) {
	entry, ok := c.casesByName[name]
	return entry, ok
}

// Cases returns all fixtures in file order.
func (c URLMaterializationCorpus) Cases() []URLMaterializationCase {
	return append([]URLMaterializationCase(nil), c.allCases...)
}

//go:embed testdata/url_materialization_cases.json
var urlMaterializationCasesJSON []byte

var (
	urlMaterializationCorpusOnce sync.Once
	urlMaterializationCorpus     URLMaterializationCorpus
	urlMaterializationCorpusErr  error
)

// LoadURLMaterializationCorpus returns the shared URL materialization fixture corpus.
func LoadURLMaterializationCorpus() (URLMaterializationCorpus, error) {
	urlMaterializationCorpusOnce.Do(func() {
		urlMaterializationCorpus, urlMaterializationCorpusErr = loadURLMaterializationCorpus()
	})
	return urlMaterializationCorpus, urlMaterializationCorpusErr
}

func loadURLMaterializationCorpus() (URLMaterializationCorpus, error) {
	var raw urlMaterializationCasesFile
	if err := json.Unmarshal(urlMaterializationCasesJSON, &raw); err != nil {
		return URLMaterializationCorpus{}, fmt.Errorf("decode url materialization cases: %w", err)
	}
	if len(raw.Cases) == 0 {
		return URLMaterializationCorpus{}, fmt.Errorf("decode url materialization cases: no cases")
	}

	casesByName := make(map[string]URLMaterializationCase, len(raw.Cases))
	for _, entry := range raw.Cases {
		if entry.Name == "" {
			return URLMaterializationCorpus{}, fmt.Errorf("decode url materialization cases: case missing name")
		}
		if entry.Expect.Outcome == "" {
			return URLMaterializationCorpus{}, fmt.Errorf("decode url materialization cases: case %q missing expect.outcome", entry.Name)
		}
		if _, exists := casesByName[entry.Name]; exists {
			return URLMaterializationCorpus{}, fmt.Errorf("decode url materialization cases: duplicate case %q", entry.Name)
		}
		casesByName[entry.Name] = entry
	}

	return URLMaterializationCorpus{
		casesByName: casesByName,
		allCases:    append([]URLMaterializationCase(nil), raw.Cases...),
	}, nil
}

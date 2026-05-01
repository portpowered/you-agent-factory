package functional_test

import (
	"testing"

	"github.com/portpowered/agent-factory/pkg/workers"
)

func loadProviderErrorCorpusForTest(t *testing.T) workers.ProviderErrorCorpus {
	t.Helper()

	corpus, err := workers.LoadProviderErrorCorpus()
	if err != nil {
		t.Fatalf("workers.LoadProviderErrorCorpus() error = %v", err)
	}
	return corpus
}

func providerErrorCorpusEntryForTest(t *testing.T, name string) workers.ProviderErrorCorpusEntry {
	t.Helper()

	entry, ok := loadProviderErrorCorpusForTest(t).Entry(name)
	if !ok {
		t.Fatalf("provider error corpus entry %q not found", name)
	}
	return entry
}

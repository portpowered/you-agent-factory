package functional_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/workers"
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

func providerErrorCorpusEntryLabel(entry workers.ProviderErrorCorpusEntry) string {
	if entry.UpstreamSourceCase == "" {
		return entry.Name
	}
	return entry.Name + " [" + entry.UpstreamSourceCase + "]"
}

func providerErrorCorpusLastErrorLine(t *testing.T, entry workers.ProviderErrorCorpusEntry) string {
	t.Helper()

	var lastMatch string
	for _, stream := range []string{entry.Stderr, entry.Stdout} {
		for _, line := range strings.Split(stream, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "ERROR:") {
				lastMatch = trimmed
			}
		}
	}
	if lastMatch == "" {
		t.Fatalf("provider error corpus entry %q contains no ERROR: line", providerErrorCorpusEntryLabel(entry))
	}
	return lastMatch
}

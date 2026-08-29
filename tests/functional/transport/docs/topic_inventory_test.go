package docs_test

import (
	"slices"
	"strings"
	"testing"

	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
)

// TestDocsTopicInventory_AliasesRemainQueryableThroughPackagedSurface proves packaged topic aliases remain public.
func TestDocsTopicInventory_AliasesRemainQueryableThroughPackagedSurface(t *testing.T) {
	entries := docscli.TopicIndexEntries()
	commands := docscli.SupportedTopicCommands()
	topics := docscli.SupportedTopics()
	if len(entries) == 0 || len(entries) != len(topics) {
		t.Fatalf("topic inventory lengths = entries %d topics %d", len(entries), len(topics))
	}

	wantAliases := map[string][]string{
		"workstations": {"workstation"},
		"batch-inputs": {"batch-work"},
		"providers":    {"acp"},
	}
	for _, entry := range entries {
		want, ok := wantAliases[entry.Name]
		if !ok {
			if len(entry.Aliases) != 0 {
				t.Fatalf("topic %q unexpectedly exposes aliases %#v", entry.Name, entry.Aliases)
			}
			continue
		}
		if !slices.Equal(entry.Aliases, want) {
			t.Fatalf("topic %q aliases = %#v, want %#v", entry.Name, entry.Aliases, want)
		}
		canonicalOutput := executeDocsCommand(t, "docs", entry.Name)
		if !strings.Contains(canonicalOutput, "# ") {
			t.Fatalf("you docs %s returned empty markdown:\n%s", entry.Name, canonicalOutput)
		}
		for _, alias := range want {
			if !slices.Contains(commands, alias) {
				t.Fatalf("SupportedTopicCommands() missing alias %q", alias)
			}
			output := executeDocsCommand(t, "docs", alias)
			if !strings.Contains(output, "# ") {
				t.Fatalf("you docs %s returned empty markdown:\n%s", alias, output)
			}
			if output != canonicalOutput {
				t.Fatalf("alias %s output differs from canonical topic %s", alias, entry.Name)
			}
		}
	}

	for _, topic := range topics {
		if !slices.Contains(commands, topic) {
			t.Fatalf("SupportedTopicCommands() missing canonical topic %q", topic)
		}
		if !slices.ContainsFunc(entries, func(entry docscli.TopicIndexEntry) bool {
			return entry.Name == topic
		}) {
			t.Fatalf("TopicIndexEntries() missing canonical topic %q", topic)
		}
		if !strings.Contains(executeDocsCommand(t, "docs", topic), "# ") {
			t.Fatalf("you docs %s returned empty markdown", topic)
		}
	}

	index := executeDocsCommand(t, "docs")
	for _, topic := range topics {
		if !strings.Contains(index, "`"+topic+"`") {
			t.Fatalf("docs index is missing canonical topic %q", topic)
		}
	}
}

func executeDocsCommand(t *testing.T, args ...string) string {
	t.Helper()
	process := documentationProcess(t)
	result := executeDocumentationCommandResult(
		t,
		process.process,
		isolatedDocumentationEnvironment(t),
		process.tempDir(t),
		args...,
	)
	if result.err != nil {
		t.Fatalf("execute root command %v: %v\nstdout:\n%s\nstderr:\n%s", args, result.err, result.stdout, result.stderr)
	}
	return result.stdout
}

package docs_test

import (
	"slices"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
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
		for _, alias := range want {
			if !slices.Contains(commands, alias) {
				t.Fatalf("SupportedTopicCommands() missing alias %q", alias)
			}
			output := executeDocsCommand(t, "docs", alias)
			if !strings.Contains(output, "# ") {
				t.Fatalf("you docs %s returned empty markdown:\n%s", alias, output)
			}
		}
	}

	for _, topic := range topics {
		if !slices.Contains(commands, topic) {
			t.Fatalf("SupportedTopicCommands() missing canonical topic %q", topic)
		}
	}
}

func executeDocsCommand(t *testing.T, args ...string) string {
	t.Helper()
	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), append([]string{"you"}, args...))
	inputs.WorkingDirectory = t.TempDir()
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("execute root command %v: %v", args, err)
	}
	return inputs.Stdout()
}

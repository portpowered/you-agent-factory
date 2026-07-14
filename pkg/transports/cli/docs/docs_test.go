package docs

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestTopicRegistry_OrdersTopicsAndClonesAliases(t *testing.T) {
	source := []topicDocument{
		{topic: TopicTemplates, description: "templates", path: "templates.md", displayOrder: 20},
		{topic: TopicConfig, description: "config", path: "config.md", displayOrder: 10, aliases: []Topic{TopicBatchWorkAlias}},
	}

	registry := newTopicRegistry(source)
	if got, want := []Topic{registry.ordered[0].topic, registry.ordered[1].topic}, []Topic{TopicConfig, TopicTemplates}; !reflect.DeepEqual(got, want) {
		t.Fatalf("registry order = %#v, want %#v", got, want)
	}

	source[1].aliases[0] = TopicWorkstationAlias
	if got, want := registry.commandToSource[string(TopicBatchWorkAlias)].topic, TopicConfig; got != want {
		t.Fatalf("alias lookup after source mutation = %q, want %q", got, want)
	}
	if slices.Contains(registry.commandToSource[string(TopicConfig)].aliases, TopicWorkstationAlias) {
		t.Fatal("registry should clone alias slices instead of sharing source storage")
	}
}

func TestTopicRegistry_RejectsDuplicateCommands(t *testing.T) {
	source := []topicDocument{
		{topic: TopicConfig, description: "config", path: "config.md", displayOrder: 10},
		{topic: TopicTemplates, description: "templates", path: "templates.md", displayOrder: 20, aliases: []Topic{TopicConfig}},
	}

	defer func() {
		if recovered := recover(); recovered == nil || !strings.Contains(fmt.Sprint(recovered), `duplicate docs topic command registration for "config"`) {
			t.Fatalf("newTopicRegistry() panic = %v, want duplicate command diagnostic", recovered)
		}
	}()
	newTopicRegistry(source)
}

func TestTopicAccessors_ReturnConsistentIndependentViews(t *testing.T) {
	topics := SupportedTopics()
	commands := SupportedTopicCommands()
	summaries := TopicSummaries()
	entries := TopicIndexEntries()
	if len(topics) == 0 || len(topics) != len(summaries) || len(topics) != len(entries) {
		t.Fatalf("topic view lengths = topics %d, summaries %d, entries %d", len(topics), len(summaries), len(entries))
	}

	wantCommands := acceptedCommandSet(t, commands)
	assertTopicViewsConsistent(t, topics, summaries, entries, wantCommands)

	topics[0] = "mutated"
	commands[0] = "mutated"
	if SupportedTopics()[0] == "mutated" || SupportedTopicCommands()[0] == "mutated" {
		t.Fatal("topic accessors exposed mutable registry storage")
	}
}

func acceptedCommandSet(t *testing.T, commands []string) map[string]bool {
	t.Helper()
	wantCommands := make(map[string]bool, len(commands))
	for _, command := range commands {
		if wantCommands[command] {
			t.Fatalf("SupportedTopicCommands() contains duplicate %q", command)
		}
		wantCommands[command] = true
	}
	return wantCommands
}

func assertTopicViewsConsistent(t *testing.T, topics []string, summaries []TopicSummary, entries []TopicIndexEntry, wantCommands map[string]bool) {
	t.Helper()
	for i, topic := range topics {
		if summaries[i].Name != topic || entries[i].Name != topic {
			t.Fatalf("topic view %d names = %q, %q, %q", i, topic, summaries[i].Name, entries[i].Name)
		}
		if summaries[i].Description == "" || entries[i].Description != summaries[i].Description {
			t.Fatalf("topic %q descriptions are inconsistent", topic)
		}
		if !wantCommands[topic] {
			t.Fatalf("canonical topic %q is missing from accepted commands", topic)
		}
		for _, alias := range entries[i].Aliases {
			if !wantCommands[alias] {
				t.Fatalf("topic %q alias %q is missing from accepted commands", topic, alias)
			}
		}
	}
}

func TestIndexMarkdown_UsesRequestedExecutableAndCanonicalSummaries(t *testing.T) {
	const cliName = "factory-cli"
	quickStart := QuickStartMarkdown(cliName)
	index := IndexMarkdown(cliName)
	if !strings.Contains(quickStart, "`factory-cli docs agents`") || !strings.HasPrefix(index, "# Docs\n\n"+quickStart) {
		t.Fatalf("index did not compose the requested executable quick start:\n%s", index)
	}
	for _, summary := range TopicSummaries() {
		line := "- `" + summary.Name + "` - " + summary.Description + " Run `factory-cli docs " + summary.Name + "`."
		if strings.Count(index, line) != 1 {
			t.Fatalf("index count for canonical summary %q = %d, want 1", summary.Name, strings.Count(index, line))
		}
	}
}

func TestMarkdown_ResolvesAliasesAndReportsUnsupportedTopics(t *testing.T) {
	canonical, err := Markdown(string(TopicBatchInputs))
	if err != nil {
		t.Fatalf("Markdown(%s): %v", TopicBatchInputs, err)
	}
	alias, err := Markdown(string(TopicBatchWorkAlias))
	if err != nil {
		t.Fatalf("Markdown(%s): %v", TopicBatchWorkAlias, err)
	}
	if canonical == "" || alias != canonical {
		t.Fatal("canonical topic and compatibility alias did not resolve to the same packaged page")
	}

	const unsupported = "not-a-topic"
	got, err := Markdown(unsupported)
	if err == nil || got != "" || !strings.Contains(err.Error(), fmt.Sprintf("unsupported docs topic %q", unsupported)) {
		t.Fatalf("Markdown(%q) = %q, %v; want unsupported-topic error", unsupported, got, err)
	}
	for _, topic := range SupportedTopics() {
		if !strings.Contains(err.Error(), topic) {
			t.Fatalf("unsupported-topic error omitted canonical topic %q: %v", topic, err)
		}
	}
}

func TestMarkdown_ReportsEmbeddedReadFailure(t *testing.T) {
	original := packagedReferenceDocs
	packagedReferenceDocs = fstest.MapFS{}
	t.Cleanup(func() { packagedReferenceDocs = original })

	got, err := Markdown(string(TopicRun))
	if err == nil || got != "" || !strings.Contains(err.Error(), `read embedded docs topic "run"`) {
		t.Fatalf("Markdown(run) = %q, %v; want embedded read error", got, err)
	}
}

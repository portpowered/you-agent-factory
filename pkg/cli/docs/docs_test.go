package docs

import (
	"reflect"
	"slices"
	"testing"
)

func TestTopicRegistry_OrdersTopicsAndClonesAliases(t *testing.T) {
	t.Parallel()

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

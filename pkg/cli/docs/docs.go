// Package docs provides the packaged markdown reference topics exposed by the
// agent-factory CLI docs surface.
package docs

import (
	"embed"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Topic is one stable docs topic name exposed by the CLI.
type Topic string

const (
	TopicAuthoringFactories Topic = "authoring-factories"
	TopicConfig             Topic = "config"
	TopicMockWorkers        Topic = "mock-workers"
	TopicRecordReplay       Topic = "record-replay"
	TopicGuards             Topic = "guards"
	TopicRelationships      Topic = "relationships"
	TopicWork               Topic = "work"
	TopicWorkstations       Topic = "workstations"
	TopicWorkers            Topic = "workers"
	TopicResources          Topic = "resources"
	TopicModels             Topic = "models"
	TopicBatchInputs        Topic = "batch-inputs"
	TopicTemplates          Topic = "templates"
)

const (
	TopicWorkstationAlias Topic = "workstation"
	TopicBatchWorkAlias   Topic = "batch-work"
)

const (
	referenceAuthoringFactoriesPath = "reference/authoring-factories.md"
	referenceConfigPath             = "reference/config.md"
	referenceMockWorkersPath        = "reference/mock-workers.md"
	referenceRecordReplayPath       = "reference/record-replay.md"
	referenceGuardsPath             = "reference/guards.md"
	referenceRelationshipsPath      = "reference/relationships.md"
	referenceWorkPath               = "reference/work.md"
	referenceWorkstationsPath       = "reference/workstations.md"
	referenceWorkersPath            = "reference/workers.md"
	referenceResourcesPath          = "reference/resources.md"
	referenceModelsPath             = "reference/models.md"
	referenceBatchInputsPath        = "reference/batch-inputs.md"
	referenceTemplatesPath          = "reference/templates.md"
)

type topicDocument struct {
	topic        Topic
	description  string
	path         string
	displayOrder int
	aliases      []Topic
}

var topicDocuments = []topicDocument{
	{topic: TopicAuthoringFactories, description: "Practical factory authoring workflow, runnable examples, mock workers, and replay.", path: referenceAuthoringFactoriesPath, displayOrder: 10},
	{topic: TopicConfig, description: "Factory configuration, work types, workers, resources, and local run options.", path: referenceConfigPath, displayOrder: 20},
	{topic: TopicMockWorkers, description: "Mock-worker runs, JSON selection contract, and deterministic accept, reject, and script outcomes.", path: referenceMockWorkersPath, displayOrder: 25},
	{topic: TopicRecordReplay, description: "Record and replay run modes, artifact paths, sensitivity, and incompatible flag combinations.", path: referenceRecordReplayPath, displayOrder: 26},
	{topic: TopicGuards, description: "Workstation, input, and factory guards, guarded LOGICAL_MOVE loop breakers, and guard attachment levels.", path: referenceGuardsPath, displayOrder: 27},
	{topic: TopicRelationships, description: "Batch DEPENDS_ON and PARENT_CHILD relations, runtime SPAWNED_BY lineage, and parent-aware guard linkage.", path: referenceRelationshipsPath, displayOrder: 28},
	{topic: TopicWork, description: "Work types, states, routing, resources, and portable factory fields.", path: referenceWorkPath, displayOrder: 30},
	{topic: TopicWorkstations, description: "Workstation kinds, route fields, runtime step behavior, and scoped execution settings.", path: referenceWorkstationsPath, displayOrder: 40, aliases: []Topic{TopicWorkstationAlias}},
	{topic: TopicWorkers, description: "Worker types, model providers, script workers, and worker configuration.", path: referenceWorkersPath, displayOrder: 50},
	{topic: TopicResources, description: "Resource capacity, bounded concurrency, and workstation resource requirements.", path: referenceResourcesPath, displayOrder: 60},
	{topic: TopicModels, description: "Local and hosted model setup for workers and CLI model commands.", path: referenceModelsPath, displayOrder: 70},
	{topic: TopicBatchInputs, description: "Batch input files, request shape, dependencies, and validation.", path: referenceBatchInputsPath, displayOrder: 80, aliases: []Topic{TopicBatchWorkAlias}},
	{topic: TopicTemplates, description: "Prompt template variables, context fields, and Go template behavior.", path: referenceTemplatesPath, displayOrder: 90},
}

var (
	//go:embed reference/*.md
	embeddedReferenceDocs embed.FS
	topicRegistry         = newTopicRegistry(topicDocuments)
)

type topicRegistryData struct {
	ordered         []topicDocument
	commandToSource map[string]topicDocument
}

// TopicSummary describes one canonical packaged docs topic for index output.
type TopicSummary struct {
	Name        string
	Description string
}

// SupportedTopics returns the fixed docs topics exposed by the packaged CLI
// docs surface in display order.
func SupportedTopics() []string {
	supportedTopics := make([]string, 0, len(topicRegistry.ordered))
	for _, doc := range topicRegistry.ordered {
		supportedTopics = append(supportedTopics, string(doc.topic))
	}
	return append([]string(nil), supportedTopics...)
}

// SupportedTopicCommands returns canonical topic names and compatibility
// aliases accepted by the CLI as subcommands.
func SupportedTopicCommands() []string {
	commands := make([]string, 0, len(topicRegistry.ordered))
	for _, doc := range topicRegistry.ordered {
		commands = append(commands, string(doc.topic))
		for _, alias := range doc.aliases {
			commands = append(commands, string(alias))
		}
	}
	return append([]string(nil), commands...)
}

// TopicSummaries returns canonical packaged docs topic metadata in display order.
func TopicSummaries() []TopicSummary {
	summaries := make([]TopicSummary, 0, len(topicRegistry.ordered))
	for _, doc := range topicRegistry.ordered {
		summaries = append(summaries, TopicSummary{
			Name:        string(doc.topic),
			Description: doc.description,
		})
	}
	return summaries
}

// IndexMarkdown returns a terminal-friendly index of packaged docs topics.
func IndexMarkdown(cliName string) string {
	var builder strings.Builder
	builder.WriteString("# Docs\n\n")
	builder.WriteString("Packaged reference topics:\n\n")
	for _, summary := range TopicSummaries() {
		builder.WriteString("- `")
		builder.WriteString(summary.Name)
		builder.WriteString("` - ")
		builder.WriteString(summary.Description)
		builder.WriteString(" Run `")
		builder.WriteString(cliName)
		builder.WriteString(" docs ")
		builder.WriteString(summary.Name)
		builder.WriteString("`.\n")
	}
	return builder.String()
}

// Markdown returns the embedded markdown page for one supported topic.
func Markdown(topic string) (string, error) {
	doc, ok := topicRegistry.commandToSource[topic]
	if !ok {
		supportedTopics := SupportedTopics()
		return "", fmt.Errorf("unsupported docs topic %q (supported: %s)", topic, strings.Join(supportedTopics, ", "))
	}

	content, err := embeddedReferenceDocs.ReadFile(doc.path)
	if err != nil {
		return "", fmt.Errorf("read embedded docs topic %q: %w", topic, err)
	}

	return string(content), nil
}

func newTopicRegistry(source []topicDocument) topicRegistryData {
	ordered := append([]topicDocument(nil), source...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].displayOrder == ordered[j].displayOrder {
			return ordered[i].topic < ordered[j].topic
		}
		return ordered[i].displayOrder < ordered[j].displayOrder
	})

	commandToSource := make(map[string]topicDocument, len(ordered))
	for _, doc := range ordered {
		registerTopicCommand(commandToSource, string(doc.topic), doc)
		for _, alias := range doc.aliases {
			registerTopicCommand(commandToSource, string(alias), doc)
		}
	}
	return topicRegistryData{
		ordered:         ordered,
		commandToSource: commandToSource,
	}
}

func registerTopicCommand(index map[string]topicDocument, command string, doc topicDocument) {
	if _, exists := index[command]; exists {
		panic(fmt.Sprintf("duplicate docs topic command registration for %q", command))
	}
	index[command] = doc.clone()
}

func (d topicDocument) clone() topicDocument {
	cloned := d
	cloned.aliases = slices.Clone(d.aliases)
	return cloned
}

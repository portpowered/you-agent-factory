// Package docs provides the packaged markdown reference topics exposed by the
// agent-factory CLI docs surface.
package docs

import (
	"embed"
	"fmt"
	"strings"
)

// Topic is one stable docs topic name exposed by the CLI.
type Topic string

const (
	TopicConfig      Topic = "config"
	TopicWorkstation Topic = "workstation"
	TopicWorkers     Topic = "workers"
	TopicResources   Topic = "resources"
	TopicModels      Topic = "models"
	TopicBatchWork   Topic = "batch-work"
	TopicTemplates   Topic = "templates"
)

const (
	referenceConfigPath      = "reference/config.md"
	referenceWorkstationPath = "reference/workstation.md"
	referenceWorkersPath     = "reference/workers.md"
	referenceResourcesPath   = "reference/resources.md"
	referenceModelsPath      = "reference/models.md"
	referenceBatchWorkPath   = "reference/batch-work.md"
	referenceTemplatesPath   = "reference/templates.md"
)

type topicDocument struct {
	topic       Topic
	description string
	path        string
}

var topicDocuments = []topicDocument{
	{topic: TopicConfig, description: "Factory configuration, work types, workers, resources, and local run options.", path: referenceConfigPath},
	{topic: TopicWorkstation, description: "Workstation state, inputs, outputs, moves, and dispatch routing.", path: referenceWorkstationPath},
	{topic: TopicWorkers, description: "Worker types, model providers, script workers, and worker configuration.", path: referenceWorkersPath},
	{topic: TopicResources, description: "Resource capacity, bounded concurrency, and workstation resource requirements.", path: referenceResourcesPath},
	{topic: TopicModels, description: "Local and hosted model setup for workers and CLI model commands.", path: referenceModelsPath},
	{topic: TopicBatchWork, description: "Batch work input files, request shape, dependencies, and validation.", path: referenceBatchWorkPath},
	{topic: TopicTemplates, description: "Prompt template variables, context fields, and Go template behavior.", path: referenceTemplatesPath},
}

var (
	//go:embed reference/*.md
	embeddedReferenceDocs embed.FS
)

// TopicSummary describes one canonical packaged docs topic for index output.
type TopicSummary struct {
	Name        string
	Description string
}

// SupportedTopics returns the fixed docs topics exposed by the packaged CLI
// docs surface in display order.
func SupportedTopics() []string {
	supportedTopics := make([]string, 0, len(topicDocuments))
	for _, doc := range topicDocuments {
		supportedTopics = append(supportedTopics, string(doc.topic))
	}
	return append([]string(nil), supportedTopics...)
}

// TopicSummaries returns canonical packaged docs topic metadata in display order.
func TopicSummaries() []TopicSummary {
	summaries := make([]TopicSummary, 0, len(topicDocuments))
	for _, doc := range topicDocuments {
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
	var path string
	for _, doc := range topicDocuments {
		if topic == string(doc.topic) {
			path = doc.path
			break
		}
	}
	if path == "" {
		supportedTopics := SupportedTopics()
		return "", fmt.Errorf("unsupported docs topic %q (supported: %s)", topic, strings.Join(supportedTopics, ", "))
	}

	content, err := embeddedReferenceDocs.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read embedded docs topic %q: %w", topic, err)
	}

	return string(content), nil
}

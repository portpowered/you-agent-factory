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
	topic Topic
	path  string
}

var topicDocuments = []topicDocument{
	{topic: TopicConfig, path: referenceConfigPath},
	{topic: TopicWorkstation, path: referenceWorkstationPath},
	{topic: TopicWorkers, path: referenceWorkersPath},
	{topic: TopicResources, path: referenceResourcesPath},
	{topic: TopicModels, path: referenceModelsPath},
	{topic: TopicBatchWork, path: referenceBatchWorkPath},
	{topic: TopicTemplates, path: referenceTemplatesPath},
}

var topicPaths = map[string]string{
	string(TopicConfig):      referenceConfigPath,
	string(TopicWorkstation): referenceWorkstationPath,
	string(TopicWorkers):     referenceWorkersPath,
	string(TopicResources):   referenceResourcesPath,
	string(TopicModels):      referenceModelsPath,
	string(TopicBatchWork):   referenceBatchWorkPath,
	string(TopicTemplates):   referenceTemplatesPath,
}

var supportedTopics = []string{
	string(TopicConfig),
	string(TopicWorkstation),
	string(TopicWorkers),
	string(TopicResources),
	string(TopicModels),
	string(TopicBatchWork),
	string(TopicTemplates),
}

var (
	//go:embed reference/*.md
	embeddedReferenceDocs embed.FS
)

// SupportedTopics returns the fixed docs topics exposed by the packaged CLI
// docs surface in display order.
func SupportedTopics() []string {
	return append([]string(nil), supportedTopics...)
}

// Markdown returns the embedded markdown page for one supported topic.
func Markdown(topic string) (string, error) {
	path, ok := topicPaths[topic]
	if !ok {
		return "", fmt.Errorf("unsupported docs topic %q (supported: %s)", topic, strings.Join(supportedTopics, ", "))
	}

	content, err := embeddedReferenceDocs.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read embedded docs topic %q: %w", topic, err)
	}

	return string(content), nil
}

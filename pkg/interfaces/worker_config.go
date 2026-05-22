package interfaces

import "time"

// WorkerConfig is the canonical worker configuration used by factory.json,
// worker AGENTS.md frontmatter, and loaded runtime config.
type WorkerConfig struct {
	Name             string           `json:"name" yaml:"name,omitempty"`
	Type             string           `json:"type" yaml:"type"`
	Model            string           `json:"model,omitempty" yaml:"model,omitempty"`
	ModelProvider    string           `json:"modelProvider,omitempty" yaml:"modelProvider,omitempty"`
	ModelLocality    string           `json:"modelLocality,omitempty" yaml:"modelLocality,omitempty"`
	ExecutorProvider string           `json:"executorProvider,omitempty" yaml:"executorProvider,omitempty"`
	Operations       []ModelOperation `json:"operations,omitempty" yaml:"operations,omitempty"`
	Command          string           `json:"command,omitempty" yaml:"command,omitempty"`
	Args             []string         `json:"args,omitempty" yaml:"args,omitempty"`
	Resources        []ResourceConfig `json:"resources,omitempty" yaml:"resources,omitempty"`
	Timeout          string           `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	StopToken        string           `json:"stopToken,omitempty" yaml:"stopToken,omitempty"`
	SkipPermissions  bool             `json:"skipPermissions,omitempty" yaml:"skipPermissions,omitempty"`
	Body             string           `json:"body,omitempty" yaml:"-"`

	// Internal-only runtime fields retained during contract cleanup.
	SessionID   string `json:"-" yaml:"-"`
	Concurrency int    `json:"-" yaml:"-"`
}

// TimeoutDuration parses Timeout as a time.Duration. It returns zero when the
// value is empty or invalid.
func (w *WorkerConfig) TimeoutDuration() time.Duration {
	if w.Timeout == "" {
		return 0
	}
	d, _ := time.ParseDuration(w.Timeout)
	return d
}

const (
	ModelLocalityLocal = "LOCAL"
	ModelLocalityCloud = "CLOUD"
)

const (
	ModelOperationContentTypeText   = "TEXT"
	ModelOperationContentTypeImage  = "IMAGE"
	ModelOperationContentTypeAudio  = "AUDIO"
	ModelOperationContentTypeJSON   = "JSON"
	ModelOperationContentTypeBinary = "BINARY"
)

// ModelOperation declares one provider-agnostic capability exposed by a model worker.
type ModelOperation struct {
	Name    string               `json:"name" yaml:"name"`
	Inputs  []ModelOperationSlot `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs []ModelOperationSlot `json:"outputs,omitempty" yaml:"outputs,omitempty"`
}

// ModelOperationSlot declares one named operation input or output slot.
type ModelOperationSlot struct {
	Name         string   `json:"name" yaml:"name"`
	ContentTypes []string `json:"contentTypes,omitempty" yaml:"contentTypes,omitempty"`
	Required     bool     `json:"required,omitempty" yaml:"required,omitempty"`
}

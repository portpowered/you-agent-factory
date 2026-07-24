package models

import (
	"errors"
	"fmt"
	"strings"
)

// RuntimeOpeningRequest contains Models-owned process input for one runtime
// scope opening. Together with RuntimeBinding / RuntimeConfig it forms the
// plain runtime-scope request vocabulary peers use without local-runtime types.
type RuntimeOpeningRequest struct {
	CacheDirectory string
}

const (
	RuntimeModelLocalityLocal  = "LOCAL"
	RuntimeModelLocalityCloud  = "CLOUD"
	RuntimeResourceTypeModel   = "MODEL"
	RuntimeWorkerTypeInference = "INFERENCE_WORKER"
	RuntimeWorkerTypeAgent     = "AGENT_WORKER"
	RuntimeWorkerTypeModel     = "MODEL_WORKER"
	RuntimeContentTypeText     = "TEXT"
	RuntimeContentTypeAudio    = "AUDIO"
)

// RuntimeConfig is the Models-owned projection of authored runtime values
// needed by catalog, asset, host, and local-execution behavior.
type RuntimeConfig struct {
	FactoryDirectory string
	BaseDirectory    string
	Workers          []RuntimeWorker
	Resources        []RuntimeResource
}

// RuntimeConfigLoader returns the current detached Models runtime projection.
type RuntimeConfigLoader func() *RuntimeConfig

// RuntimeWorker carries only Worker fields used by Models.
type RuntimeWorker struct {
	Name          string
	Type          string
	Model         string
	ModelProvider string
	ModelLocality string
	Command       string
	Args          []string
	Operations    []RuntimeOperation
	Resources     []RuntimeResource
}

// RuntimeResource carries only authored resource fields used by Models.
type RuntimeResource struct {
	ID         string
	Name       string
	Type       string
	Capacity   int
	Model      string
	Backend    string
	LoadPolicy string
	Provider   string
}

// RuntimeOperation describes one model capability in the Models projection.
type RuntimeOperation struct {
	Name    string
	Inputs  []RuntimeOperationSlot
	Outputs []RuntimeOperationSlot
}

// RuntimeOperationSlot describes one model-operation input or output.
type RuntimeOperationSlot struct {
	Name         string
	ContentTypes []string
	Required     bool
}

// Worker returns a detached worker projection by authored name.
func (c *RuntimeConfig) Worker(name string) (*RuntimeWorker, bool) {
	if c == nil {
		return nil, false
	}
	for i := range c.Workers {
		if c.Workers[i].Name != name {
			continue
		}
		worker := c.Workers[i].Clone()
		return &worker, true
	}
	return nil, false
}

// Clone detaches all slice fields in a runtime Worker projection.
func (worker RuntimeWorker) Clone() RuntimeWorker {
	worker.Args = append([]string(nil), worker.Args...)
	worker.Resources = append([]RuntimeResource(nil), worker.Resources...)
	worker.Operations = cloneRuntimeOperations(worker.Operations)
	return worker
}

func cloneRuntimeOperations(operations []RuntimeOperation) []RuntimeOperation {
	cloned := make([]RuntimeOperation, len(operations))
	for i := range operations {
		cloned[i] = operations[i]
		cloned[i].Inputs = cloneRuntimeOperationSlots(operations[i].Inputs)
		cloned[i].Outputs = cloneRuntimeOperationSlots(operations[i].Outputs)
	}
	return cloned
}

func cloneRuntimeOperationSlots(slots []RuntimeOperationSlot) []RuntimeOperationSlot {
	cloned := make([]RuntimeOperationSlot, len(slots))
	for i := range slots {
		cloned[i] = slots[i]
		cloned[i].ContentTypes = append([]string(nil), slots[i].ContentTypes...)
	}
	return cloned
}

// UsesManagedRuntime reports whether Models owns local capacity for this Worker.
func (worker RuntimeWorker) UsesManagedRuntime() bool {
	return (worker.IsInference() || worker.IsAgent()) &&
		strings.TrimSpace(worker.ModelLocality) == RuntimeModelLocalityLocal
}

// IsInference recognizes current and compatibility inference
// Worker taxonomy values without importing Factory Definitions.
func (worker RuntimeWorker) IsInference() bool {
	switch strings.TrimSpace(worker.Type) {
	case RuntimeWorkerTypeInference, RuntimeWorkerTypeModel:
		return true
	default:
		return false
	}
}

// IsAgent recognizes the agent Worker taxonomy value.
func (worker RuntimeWorker) IsAgent() bool {
	return strings.TrimSpace(worker.Type) == RuntimeWorkerTypeAgent
}

// ErrInvalidRuntimeBinding classifies missing or invalid runtime-scope inputs
// supplied to ForRuntime. Peers fail closed on this typed outcome without
// importing local-runtime construction or process-launcher types.
var ErrInvalidRuntimeBinding = errors.New("models runtime binding is invalid")

// ValidateRuntimeBinding checks the plain runtime-scope inputs required to bind
// a constructed Models service to one Factory Session. It does not start host
// processes or touch local-runtime implementation packages.
func ValidateRuntimeBinding(binding RuntimeBinding) error {
	if strings.TrimSpace(binding.CacheDirectory) == "" {
		return fmt.Errorf("%w: cache directory is required", ErrInvalidRuntimeBinding)
	}
	if binding.RuntimeConfig == nil {
		return fmt.Errorf("%w: runtime configuration lookup is required", ErrInvalidRuntimeBinding)
	}
	return nil
}

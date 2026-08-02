package models

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrRuntimeScopeInvalid reports an empty or malformed runtime-scope
	// reference.
	ErrRuntimeScopeInvalid = errors.New("models runtime scope is invalid")
	// ErrRuntimeScopeStale reports a well-formed reference that no longer names
	// a scope known by the issuing Models service.
	ErrRuntimeScopeStale = errors.New("models runtime scope is stale")
	// ErrRuntimeScopeClosed reports a repeated close or use of a scope that was
	// explicitly closed.
	ErrRuntimeScopeClosed = errors.New("models runtime scope is closed")
	// ErrRuntimeScopeForeign reports a reference issued by another Models
	// service authority.
	ErrRuntimeScopeForeign = errors.New("models runtime scope is foreign")
)

// RuntimeScopeRef is an opaque Models-owned runtime-scope reference. Peers may
// compare, serialize, and carry it, but its representation and ownership rules
// remain private to Models implementations.
type RuntimeScopeRef struct {
	value string
}

// Parse restores an opaque reference received from a trusted boundary. Call it
// on the zero value. Parsing validates only that the serialized value is
// non-empty; the Models service classifies stale, closed, or foreign ownership
// on use.
func (RuntimeScopeRef) Parse(value string) (RuntimeScopeRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return RuntimeScopeRef{}, ErrRuntimeScopeInvalid
	}
	return RuntimeScopeRef{value: value}, nil
}

// String returns the opaque serialized value without exposing its internal
// ownership or identity layout.
func (ref RuntimeScopeRef) String() string {
	return ref.value
}

// IsZero reports whether no runtime-scope reference was supplied.
func (ref RuntimeScopeRef) IsZero() bool {
	return strings.TrimSpace(ref.value) == ""
}

// RuntimeScopeConfig contains only detached Models configuration registered
// for one scope. It deliberately excludes loaders, services, processes,
// storage handles, clocks, HTTP clients, and concrete runtime dependencies.
type RuntimeScopeConfig struct {
	// CacheDirectory may be absolute or a valid relative managed-cache
	// directory. Models resolves valid relative values against the process
	// working directory before secure filesystem effects use them.
	CacheDirectory string
	Runtime        RuntimeConfig
}

// Clone returns a detached copy safe for a Models implementation to retain.
func (config RuntimeScopeConfig) Clone() RuntimeScopeConfig {
	cloned := config
	cloned.Runtime.Workers = make([]RuntimeWorker, len(config.Runtime.Workers))
	for i := range config.Runtime.Workers {
		cloned.Runtime.Workers[i] = config.Runtime.Workers[i].Clone()
	}
	cloned.Runtime.Resources = append([]RuntimeResource(nil), config.Runtime.Resources...)
	return cloned
}

// OpenRuntimeScopeRequest asks the process-scoped Models service to register
// detached configuration without constructing runtime machinery.
type OpenRuntimeScopeRequest struct {
	Config RuntimeScopeConfig
}

// OpenRuntimeScopeResult identifies the registered configuration only by an
// opaque Models-owned reference.
type OpenRuntimeScopeResult struct {
	Scope RuntimeScopeRef
}

// CloseRuntimeScopeRequest identifies the scope to close.
type CloseRuntimeScopeRequest struct {
	Scope RuntimeScopeRef
}

// CloseRuntimeScopeResult confirms which scope was closed.
type CloseRuntimeScopeResult struct {
	Scope  RuntimeScopeRef
	Closed bool
}

// RuntimeOpeningRequest is retained as the legacy ForRuntime input projection.
//
// Deprecated: use OpenRuntimeScopeRequest with detached RuntimeScopeConfig.
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
// a constructed Models service to one Factory Session. CacheDirectory may be
// empty; the asset puller resolves a default managed-model cache root at use
// time. RuntimeConfig lookup is required. Validation does not start host
// processes or touch local-runtime implementation packages.
func ValidateRuntimeBinding(binding RuntimeBinding) error {
	if binding.RuntimeConfig == nil {
		return fmt.Errorf("%w: runtime configuration lookup is required", ErrInvalidRuntimeBinding)
	}
	return nil
}

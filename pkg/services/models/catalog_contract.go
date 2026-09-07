package models

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnsupportedOperation classifies a catalog or invocation path that Models
// does not support for the requested peer operation.
var ErrUnsupportedOperation = errors.New("model invocation operation is not supported")

// ErrUnavailable classifies catalog list/get when Models cannot serve the
// catalog (for example missing runtime scope). It is distinct from ErrNotFound
// and ErrUnsupportedOperation so peers can branch on typed outcomes.
var ErrUnavailable = errors.New("models catalog is unavailable")

// Status names catalog readiness for one model summary/detail.
type Status string

const (
	StatusReady       Status = "READY"
	StatusUnavailable Status = "UNAVAILABLE"
)

// LoadState names whether a model runtime is loaded for catalog projection.
type LoadState string

const (
	LoadStateUnloaded      LoadState = "UNLOADED"
	LoadStateNotApplicable LoadState = "NOT_APPLICABLE"
)

// ListModelsRequest is the plain catalog list request vocabulary. List currently
// takes no filters; peers use this type without nested catalog assemblers.
type ListModelsRequest struct {
	Scope RuntimeScopeRef
}

// GetModelRequest is the plain catalog get request. Peers identify a model by
// Name and may require one configured operation without importing
// models/internal/catalog.
type GetModelRequest struct {
	Scope     RuntimeScopeRef
	Name      string
	Operation string
}

// Validate checks request fields whose validity does not depend on private
// scope ownership or catalog state.
func (request GetModelRequest) Validate() error {
	if strings.TrimSpace(request.Name) == "" {
		return fmt.Errorf("%w: empty model name", ErrNotFound)
	}
	return nil
}

// ValidateGetModelRequest checks the plain get-model request. Empty names fail
// closed as ErrNotFound without touching nested catalog implementation.
//
// Deprecated: call GetModelRequest.Validate.
func ValidateGetModelRequest(request GetModelRequest) error {
	return request.Validate()
}

// GetModelReadinessRequest identifies the scoped model whose current readiness
// facts a peer needs.
type GetModelReadinessRequest struct {
	Scope     RuntimeScopeRef
	Name      string
	Operation string
}

// Validate checks request fields whose validity does not depend on private
// scope ownership or catalog state.
func (request GetModelReadinessRequest) Validate() error {
	if strings.TrimSpace(request.Name) == "" {
		return fmt.Errorf("%w: empty model name", ErrNotFound)
	}
	return nil
}

// ResourceSummary describes one resource attached to a catalog model summary.
type ResourceSummary struct {
	Name       string
	Type       string
	Capacity   int
	Model      *string
	Backend    *string
	LoadPolicy *string
	Provider   *string
}

// Capability describes one worker capability projected onto a catalog detail.
type Capability struct {
	Worker           string
	ProviderLocality Locality
	ModelProvider    *string
	Operations       []Operation
	ResourceNames    []string
}

// SourceMetadata describes a configured model source without exposing source
// resolvers, download clients, cache records, or filesystem locations.
type SourceMetadata struct {
	Provider  string
	Reference string
	Revision  string
}

// Summary is the Models-owned catalog list/get summary value, including
// Status/LoadState and ManagedRuntime readiness vocabulary.
type Summary struct {
	Name             string
	ProviderLocality Locality
	Status           Status
	LoadState        LoadState
	Operations       []Operation
	Modalities       []string
	Resources        []ResourceSummary
	ManagedRuntime   Runtime
}

// Clone returns a detached catalog summary.
func (summary Summary) Clone() Summary {
	summary.Operations = cloneOperations(summary.Operations)
	summary.Modalities = append([]string(nil), summary.Modalities...)
	summary.Resources = cloneResourceSummaries(summary.Resources)
	summary.ManagedRuntime = summary.ManagedRuntime.Clone()
	return summary
}

// Detail is the Models-owned catalog get result value.
type Detail struct {
	Summary
	Capabilities []Capability
	Sources      []SourceMetadata
	Diagnostics  map[string]string
}

// LoadPolicy describes when a managed model backend may be loaded.
type LoadPolicy string

const (
	// LoadPolicyOnDemand keeps the model unloaded until an invocation needs it.
	LoadPolicyOnDemand LoadPolicy = "ON_DEMAND"
	// LoadPolicyKeepWarm retains a ready backend after its last lease until
	// explicit stop or resource pressure requires eviction.
	LoadPolicyKeepWarm LoadPolicy = "KEEP_WARM"
)

const (
	BuiltInModelNameLLM   = "llm"
	BuiltInModelNameASR   = "asr"
	BuiltInModelNameTTS   = "tts"
	BuiltInModelNameEmbed = "embed"
)

const (
	builtInLLMSource   = "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3"
	builtInASRSource   = "hf://ggerganov/whisper.cpp/ggml-base.en.bin@5359861c739e955e79d9a303bcbc70fb988958b1"
	builtInTTSSource   = "hf://mudler/vibevoice.cpp-models/vibevoice-realtime-0.5B-q8_0.gguf@a67807e65e3002e187179a856e96043f75060bc9"
	builtInEmbedSource = "hf://Qwen/Qwen3-Embedding-0.6B-GGUF/Qwen3-Embedding-0.6B-f16.gguf@370f27d7550e0def9b39c1f16d3fbaa13aa67728"
)

// ModelDefinition describes one configured model name without resolving its
// source, inspecting a cache, or starting a backend. Operations are the
// canonical provider-neutral contracts exposed by the Models root.
type ModelDefinition struct {
	Name       string
	Source     string
	Backend    string
	LoadPolicy LoadPolicy
	Operations []Operation
}

// BuiltInModelDefinition is the descriptive alias used by catalog consumers.
type BuiltInModelDefinition = ModelDefinition

// Clone returns a detached model definition.
func (definition ModelDefinition) Clone() ModelDefinition {
	definition.Operations = cloneOperations(definition.Operations)
	return definition
}

// BuiltInCatalog is the stateless Models-root value that publishes the
// canonical built-in model definitions.
type BuiltInCatalog struct{}

// ModelDefinitions returns the canonical built-in model definitions in stable
// order. Every call returns detached values so callers cannot mutate shared
// catalog state.
func (BuiltInCatalog) ModelDefinitions() []ModelDefinition {
	return []ModelDefinition{
		builtInModelDefinition(BuiltInModelNameLLM, builtInLLMSource, "localai-llamacpp", OperationOMNI),
		builtInModelDefinition(BuiltInModelNameASR, builtInASRSource, "localai-whisper", OperationASR),
		builtInModelDefinition(BuiltInModelNameTTS, builtInTTSSource, "localai-vibevoice", OperationTTS),
		builtInModelDefinition(BuiltInModelNameEmbed, builtInEmbedSource, "localai-llamacpp", OperationEMBED),
	}
}

// ModelCatalog returns the built-in definitions keyed by their public model
// names. The returned map and every nested definition are detached.
func (catalog BuiltInCatalog) ModelCatalog() map[string]ModelDefinition {
	definitions := catalog.ModelDefinitions()
	models := make(map[string]ModelDefinition, len(definitions))
	for _, definition := range definitions {
		models[definition.Name] = definition.Clone()
	}
	return models
}

// ModelDefinitionFor returns one detached built-in definition by name. Names
// are case-insensitive at this value-only lookup boundary.
func (catalog BuiltInCatalog) ModelDefinitionFor(name string) (ModelDefinition, bool) {
	canonicalName := strings.ToLower(strings.TrimSpace(name))
	for _, definition := range catalog.ModelDefinitions() {
		if definition.Name == canonicalName {
			return definition.Clone(), true
		}
	}
	return ModelDefinition{}, false
}

func builtInModelDefinition(name, source, backend, operationName string) ModelDefinition {
	operation, _ := (GenericOperationCatalog{}).GenericOperationContract(operationName)
	return ModelDefinition{
		Name:       name,
		Source:     source,
		Backend:    backend,
		LoadPolicy: LoadPolicyOnDemand,
		Operations: []Operation{operation},
	}
}

// Clone returns a detached catalog detail.
func (detail Detail) Clone() Detail {
	detail.Summary = detail.Summary.Clone()
	capabilities := detail.Capabilities
	detail.Capabilities = make([]Capability, len(capabilities))
	for i, capability := range capabilities {
		detail.Capabilities[i] = capability
		detail.Capabilities[i].ModelProvider = cloneStringPointer(capability.ModelProvider)
		detail.Capabilities[i].Operations = cloneOperations(capability.Operations)
		detail.Capabilities[i].ResourceNames = append([]string(nil), capability.ResourceNames...)
	}
	detail.Sources = append([]SourceMetadata(nil), detail.Sources...)
	detail.Diagnostics = cloneStringMap(detail.Diagnostics)
	return detail
}

func cloneResourceSummaries(resources []ResourceSummary) []ResourceSummary {
	cloned := make([]ResourceSummary, len(resources))
	for i, resource := range resources {
		cloned[i] = resource
		cloned[i].Model = cloneStringPointer(resource.Model)
		cloned[i].Backend = cloneStringPointer(resource.Backend)
		cloned[i].LoadPolicy = cloneStringPointer(resource.LoadPolicy)
		cloned[i].Provider = cloneStringPointer(resource.Provider)
	}
	return cloned
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

// Entry pairs summary and detail for one catalog model.
type Entry struct {
	Summary Summary
	Detail  Detail
}

// List is the Models-owned catalog list result.
type List struct {
	Results []Summary
}

// ListModelsResult is the detached result of one scoped catalog list.
type ListModelsResult struct {
	Models []Summary
}

// GetModelResult is the detached result of one scoped catalog lookup.
type GetModelResult struct {
	Model Detail
}

// GetModelReadinessResult is the detached result of one scoped readiness
// lookup.
type GetModelReadinessResult struct {
	ModelName string
	Readiness Runtime
}

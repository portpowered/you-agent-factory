package interfaces

import (
	"encoding/json"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

const (
	OrchestratorKindPetri       = "PETRI"
	OrchestratorKindJavaScript  = "JAVASCRIPT"
	OrchestratorInlineEncoding  = "utf-8"
)

// FactoryOrchestratorConfig is the authored orchestrator identity for one factory.
type FactoryOrchestratorConfig struct {
	Kind       string                               `json:"kind"`
	Petri      *FactoryOrchestratorPetriConfig      `json:"petri,omitempty"`
	JavaScript *FactoryOrchestratorJavaScriptConfig `json:"javascript,omitempty"`
}

// FactoryOrchestratorPetriConfig carries Petri-specific orchestrator options.
type FactoryOrchestratorPetriConfig struct{}

// FactoryOrchestratorJavaScriptConfig carries JavaScript workflow source identity and policy.
type FactoryOrchestratorJavaScriptConfig struct {
	Dialect       string                                      `json:"dialect,omitempty"`
	SourceRef     string                                      `json:"sourceRef,omitempty"`
	InlineSource  *FactoryOrchestratorJavaScriptInlineSource  `json:"inlineSource,omitempty"`
	SourceHash    string                                      `json:"sourceHash,omitempty"`
	Entrypoint    string                                      `json:"entrypoint,omitempty"`
	Metadata      map[string]string                           `json:"metadata,omitempty"`
	ArgsSchema    json.RawMessage                             `json:"argsSchema,omitempty"`
	DefaultPolicy json.RawMessage                             `json:"defaultPolicy,omitempty"`
}

// FactoryOrchestratorJavaScriptInlineSource carries inline workflow source text.
type FactoryOrchestratorJavaScriptInlineSource struct {
	Encoding string `json:"encoding"`
	Inline   string `json:"inline"`
}

// EffectiveOrchestratorKind returns the resolved orchestrator kind for a factory.
// Missing orchestrator blocks default to PETRI for compatibility with legacy Petri factories.
func EffectiveOrchestratorKind(cfg *FactoryConfig) string {
	if cfg == nil || cfg.Orchestrator == nil {
		return OrchestratorKindPetri
	}
	kind := strings.TrimSpace(cfg.Orchestrator.Kind)
	if kind == "" {
		return OrchestratorKindPetri
	}
	return kind
}

// IsJavaScriptOrchestratorFactory reports whether the factory resolves to a JavaScript orchestrator.
func IsJavaScriptOrchestratorFactory(cfg *FactoryConfig) bool {
	return EffectiveOrchestratorKind(cfg) == OrchestratorKindJavaScript
}

// IsPetriOrchestratorFactory reports whether the factory resolves to a Petri orchestrator.
func IsPetriOrchestratorFactory(cfg *FactoryConfig) bool {
	return EffectiveOrchestratorKind(cfg) == OrchestratorKindPetri
}

// StrictPublicFactoryOrchestratorKind canonicalizes supported orchestrator kinds.
func StrictPublicFactoryOrchestratorKind(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryOrchestratorKindAliases, false)
}

// PermissivePublicFactoryOrchestratorKind canonicalizes supported orchestrator kinds and preserves unknown values.
func PermissivePublicFactoryOrchestratorKind(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryOrchestratorKindAliases, true)
}

var publicFactoryOrchestratorKindAliases = map[string]string{
	OrchestratorKindPetri:      OrchestratorKindPetri,
	OrchestratorKindJavaScript: OrchestratorKindJavaScript,
}

// GeneratedPublicFactoryOrchestratorKind returns the generated orchestrator kind enum.
func GeneratedPublicFactoryOrchestratorKind(kind string) factoryapi.FactoryOrchestratorKind {
	return factoryapi.FactoryOrchestratorKind(StrictPublicFactoryOrchestratorKind(kind))
}

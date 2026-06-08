package config

import (
	"encoding/json"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func orchestratorInternalFromAPI(value *factoryapi.FactoryOrchestrator) (*interfaces.FactoryOrchestratorConfig, error) {
	if value == nil {
		return nil, nil
	}
	kind := interfaces.StrictPublicFactoryOrchestratorKind(string(value.Kind))
	if kind == "" {
		return &interfaces.FactoryOrchestratorConfig{Kind: string(value.Kind)}, nil
	}
	cfg := &interfaces.FactoryOrchestratorConfig{
		Kind: kind,
	}
	if value.Petri != nil {
		cfg.Petri = &interfaces.FactoryOrchestratorPetriConfig{}
	}
	if value.Javascript != nil {
		jsCfg, err := orchestratorJavaScriptInternalFromAPI(*value.Javascript)
		if err != nil {
			return nil, err
		}
		cfg.JavaScript = jsCfg
	}
	return cfg, nil
}

func orchestratorJavaScriptInternalFromAPI(value factoryapi.FactoryOrchestratorJavaScriptConfig) (*interfaces.FactoryOrchestratorJavaScriptConfig, error) {
	cfg := &interfaces.FactoryOrchestratorJavaScriptConfig{
		Dialect:    stringValue(value.Dialect),
		SourceRef:  stringValue(value.SourceRef),
		SourceHash: stringValue(value.SourceHash),
		Entrypoint: stringValue(value.Entrypoint),
	}
	if value.Metadata != nil {
		cfg.Metadata = map[string]string(*value.Metadata)
	}
	if value.InlineSource != nil {
		cfg.InlineSource = &interfaces.FactoryOrchestratorJavaScriptInlineSource{
			Encoding: string(value.InlineSource.Encoding),
			Inline:   value.InlineSource.Inline,
		}
	}
	if value.ArgsSchema != nil {
		raw, err := json.Marshal(value.ArgsSchema)
		if err != nil {
			return nil, err
		}
		cfg.ArgsSchema = raw
	}
	if value.DefaultPolicy != nil {
		raw, err := json.Marshal(value.DefaultPolicy)
		if err != nil {
			return nil, err
		}
		cfg.DefaultPolicy = raw
	}
	return cfg, nil
}

func orchestratorAPIFromInternal(cfg *interfaces.FactoryConfig) *factoryapi.FactoryOrchestrator {
	if cfg == nil || cfg.Orchestrator == nil {
		return nil
	}
	kind := interfaces.EffectiveOrchestratorKind(cfg)
	apiKind := interfaces.GeneratedPublicFactoryOrchestratorKind(kind)
	result := &factoryapi.FactoryOrchestrator{
		Kind: apiKind,
	}
	if cfg.Orchestrator.Petri != nil || kind == interfaces.OrchestratorKindPetri {
		result.Petri = &factoryapi.FactoryOrchestratorPetriConfig{}
	}
	if cfg.Orchestrator.JavaScript != nil {
		result.Javascript = orchestratorJavaScriptAPIFromInternal(cfg.Orchestrator.JavaScript)
	}
	return result
}

// ProjectEffectiveOrchestratorForAPIRead fills the compatibility PETRI orchestrator
// projection when a factory has no authored orchestrator block.
func ProjectEffectiveOrchestratorForAPIRead(api factoryapi.Factory, cfg *interfaces.FactoryConfig) factoryapi.Factory {
	if api.Orchestrator != nil {
		return api
	}
	if interfaces.EffectiveOrchestratorKind(cfg) == interfaces.OrchestratorKindPetri {
		api.Orchestrator = defaultPetriOrchestratorAPI()
	}
	return api
}

func defaultPetriOrchestratorAPI() *factoryapi.FactoryOrchestrator {
	kind := factoryapi.PETRI
	return &factoryapi.FactoryOrchestrator{
		Kind:  kind,
		Petri: &factoryapi.FactoryOrchestratorPetriConfig{},
	}
}

func orchestratorJavaScriptAPIFromInternal(cfg *interfaces.FactoryOrchestratorJavaScriptConfig) *factoryapi.FactoryOrchestratorJavaScriptConfig {
	if cfg == nil {
		return nil
	}
	result := &factoryapi.FactoryOrchestratorJavaScriptConfig{
		Dialect:    stringPtrIfNotEmpty(cfg.Dialect),
		SourceRef:  stringPtrIfNotEmpty(cfg.SourceRef),
		SourceHash: stringPtrIfNotEmpty(cfg.SourceHash),
		Entrypoint: stringPtrIfNotEmpty(cfg.Entrypoint),
	}
	if len(cfg.Metadata) > 0 {
		metadata := factoryapi.StringMap(cfg.Metadata)
		result.Metadata = &metadata
	}
	if cfg.InlineSource != nil {
		result.InlineSource = &factoryapi.FactoryOrchestratorJavaScriptInlineSource{
			Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncoding(cfg.InlineSource.Encoding),
			Inline:   cfg.InlineSource.Inline,
		}
	}
	if len(cfg.ArgsSchema) > 0 {
		var argsSchema map[string]any
		if err := json.Unmarshal(cfg.ArgsSchema, &argsSchema); err == nil {
			result.ArgsSchema = &argsSchema
		}
	}
	if len(cfg.DefaultPolicy) > 0 {
		var defaultPolicy map[string]any
		if err := json.Unmarshal(cfg.DefaultPolicy, &defaultPolicy); err == nil {
			result.DefaultPolicy = &defaultPolicy
		}
	}
	return result
}

func isDefaultPetriOrchestratorAPI(value *factoryapi.FactoryOrchestrator) bool {
	if value == nil {
		return true
	}
	if value.Kind != factoryapi.PETRI {
		return false
	}
	if value.Javascript != nil {
		return false
	}
	if value.Petri == nil {
		return true
	}
	return true
}

func cloneFactoryOrchestratorConfig(cfg *interfaces.FactoryOrchestratorConfig) *interfaces.FactoryOrchestratorConfig {
	if cfg == nil {
		return nil
	}
	cloned := &interfaces.FactoryOrchestratorConfig{
		Kind: cfg.Kind,
	}
	if cfg.Petri != nil {
		cloned.Petri = &interfaces.FactoryOrchestratorPetriConfig{}
	}
	if cfg.JavaScript != nil {
		js := *cfg.JavaScript
		if cfg.JavaScript.InlineSource != nil {
			inline := *cfg.JavaScript.InlineSource
			js.InlineSource = &inline
		}
		if len(cfg.JavaScript.Metadata) > 0 {
			js.Metadata = make(map[string]string, len(cfg.JavaScript.Metadata))
			for key, value := range cfg.JavaScript.Metadata {
				js.Metadata[key] = value
			}
		}
		if len(cfg.JavaScript.ArgsSchema) > 0 {
			js.ArgsSchema = append(json.RawMessage(nil), cfg.JavaScript.ArgsSchema...)
		}
		if len(cfg.JavaScript.DefaultPolicy) > 0 {
			js.DefaultPolicy = append(json.RawMessage(nil), cfg.JavaScript.DefaultPolicy...)
		}
		cloned.JavaScript = &js
	}
	return cloned
}

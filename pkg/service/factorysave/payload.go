package factorysave

import (
	"encoding/json"
	"fmt"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

func prepareEditableFactoryPersistView(
	segment string,
	factory factoryapi.Factory,
) (*configpersist.PreparedFactoryLayoutPayload, error) {
	sanitized := stripEphemeralFactoryResponseFields(factory)
	sanitized.Version = nil
	raw, err := json.Marshal(sanitized)
	if err != nil {
		return nil, fmt.Errorf("marshal editable factory payload: %w", err)
	}
	return configpersist.PrepareFactoryLayoutPayload(segment, raw)
}

func persistPayloadFromView(
	view *configpersist.PreparedFactoryLayoutPayload,
	version factoryapi.HybridLogicalTimestamp,
) (*configpersist.PreparedFactoryLayoutPayload, error) {
	if view == nil {
		return nil, fmt.Errorf("persist factory view is required")
	}
	versioned, err := withCanonicalFactoryVersion(view.Canonical, version)
	if err != nil {
		return nil, err
	}
	return &configpersist.PreparedFactoryLayoutPayload{
		Config:         view.Config,
		Canonical:      versioned,
		LayoutOutcomes: view.LayoutOutcomes,
	}, nil
}

func withCanonicalFactoryVersion(
	canonical []byte,
	version factoryapi.HybridLogicalTimestamp,
) ([]byte, error) {
	var decoded map[string]any
	if err := json.Unmarshal(canonical, &decoded); err != nil {
		return nil, fmt.Errorf("unmarshal canonical factory payload: %w", err)
	}
	decoded["version"] = map[string]any{
		"logical":  version.Logical.Int64(),
		"physical": version.Physical.UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("marshal persisted factory payload: %w", err)
	}
	return payload, nil
}

func preparePersistedFactoryPayload(
	segment string,
	factory factoryapi.Factory,
	version factoryapi.HybridLogicalTimestamp,
) (*configpersist.PreparedFactoryLayoutPayload, error) {
	view, err := prepareEditableFactoryPersistView(segment, factory)
	if err != nil {
		return nil, err
	}
	return persistPayloadFromView(view, version)
}

func stripEphemeralFactoryResponseFields(factory factoryapi.Factory) factoryapi.Factory {
	factory.LayoutOutcomes = nil
	return factory
}

func withLayoutOutcomes(factory factoryapi.Factory, outcomes []factoryvalidation.Target) factoryapi.Factory {
	if len(outcomes) == 0 {
		factory.LayoutOutcomes = nil
		return factory
	}
	targets := factoryvalidation.ToValidationTargets(outcomes)
	factory.LayoutOutcomes = &targets
	return factory
}

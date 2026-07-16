package factorydefinition

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// PrepareEditableFactoryPersistView normalizes one editable factory definition
// into the durable split-layout persist view without optimistic-concurrency metadata.
func (s *Service) PrepareEditableFactoryPersistView(
	segment string,
	factory *interfaces.FactorySnapshot,
) (*configpersist.PreparedFactoryLayoutPayload, error) {
	if s == nil {
		return nil, fmt.Errorf("factory definition service is required")
	}
	return prepareEditableFactoryPersistView(segment, factory)
}

// PersistPayloadFromView stamps version metadata onto a prepared persist view.
func (s *Service) PersistPayloadFromView(
	view *configpersist.PreparedFactoryLayoutPayload,
	version interfaces.FactoryVersion,
) (*factoryconfig.PreparedFactoryLayoutPayload, error) {
	if s == nil {
		return nil, fmt.Errorf("factory definition service is required")
	}
	return persistPayloadFromView(view, version)
}

// PreparePersistedFactoryPayload serializes one editable factory definition into
// the durable split-layout payload shape, including version metadata.
func (s *Service) PreparePersistedFactoryPayload(
	segment string,
	factory *interfaces.FactorySnapshot,
	version interfaces.FactoryVersion,
) (*factoryconfig.PreparedFactoryLayoutPayload, error) {
	if s == nil {
		return nil, fmt.Errorf("factory definition service is required")
	}
	view, err := prepareEditableFactoryPersistView(segment, factory)
	if err != nil {
		return nil, err
	}
	return persistPayloadFromView(view, version)
}

func prepareEditableFactoryPersistView(
	segment string,
	factory *interfaces.FactorySnapshot,
) (*configpersist.PreparedFactoryLayoutPayload, error) {
	if factory == nil {
		return nil, fmt.Errorf("editable factory snapshot is required")
	}
	var editable map[string]any
	if err := factory.Decode(&editable); err != nil {
		return nil, fmt.Errorf("decode editable factory snapshot: %w", err)
	}
	delete(editable, "version")
	raw, err := json.Marshal(editable)
	if err != nil {
		return nil, fmt.Errorf("marshal editable factory payload: %w", err)
	}
	return configpersist.PrepareFactoryLayoutPayload(segment, raw)
}

func persistPayloadFromView(
	view *configpersist.PreparedFactoryLayoutPayload,
	version interfaces.FactoryVersion,
) (*factoryconfig.PreparedFactoryLayoutPayload, error) {
	if view == nil {
		return nil, fmt.Errorf("persist factory view is required")
	}
	versioned, err := withCanonicalFactoryVersion(view.Canonical, version)
	if err != nil {
		return nil, err
	}
	return &factoryconfig.PreparedFactoryLayoutPayload{
		Config:    view.Config,
		Canonical: versioned,
	}, nil
}

func withCanonicalFactoryVersion(
	canonical []byte,
	version interfaces.FactoryVersion,
) ([]byte, error) {
	var decoded map[string]any
	if err := json.Unmarshal(canonical, &decoded); err != nil {
		return nil, fmt.Errorf("unmarshal canonical factory payload: %w", err)
	}
	decoded["version"] = map[string]any{
		"logical":  strconv.FormatInt(version.Logical, 10),
		"physical": version.Physical.UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("marshal persisted factory payload: %w", err)
	}
	return payload, nil
}

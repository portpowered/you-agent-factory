package factorysave

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
)

func prepareEditableFactoryPersistView(
	segment string,
	factory factoryapi.Factory,
) (*configpersist.PreparedFactoryLayoutPayload, error) {
	factory.Version = nil
	raw, err := json.Marshal(factory)
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
		Config:    view.Config,
		Canonical: versioned,
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
		"logical":  strconv.FormatInt(version.Logical.Int64(), 10),
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

package factorydefinition

import (
	"context"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// Reader is the generated-contract definition read boundary.
type Reader interface {
	GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error)
}

// Saver is the generated-contract definition persistence boundary.
type Saver interface {
	Save(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error)
}

// API composes definition reads and saves without routing through a runtime host.
type API struct {
	reader Reader
	saver  Saver
}

var _ apisurface.FactorySaveAPI = (*API)(nil)

// NewAPI constructs the transport-facing Factory Definition service.
func NewAPI(reader Reader, saver Saver) *API {
	return &API{reader: reader, saver: saver}
}

func (a *API) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	if a == nil || a.reader == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	return a.reader.GetCurrentFactoryForSession(ctx, sessionID)
}

func (a *API) SaveFactoryForSession(ctx context.Context, sessionID string, mode factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
	if a == nil || a.saver == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	return a.saver.Save(ctx, sessionID, mode, request)
}

func (a *API) SaveCurrentFactoryForSession(ctx context.Context, sessionID string, request factoryapi.Factory) (factoryapi.Factory, error) {
	return a.SaveFactoryForSession(ctx, sessionID, factoryapi.FactorySaveModeReplaceCurrent, request)
}

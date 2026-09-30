package acp

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// FactorySessionStartResolver resolves a canonical Factory Sessions start
// request for a factory target without performing any I/O.
type FactorySessionStartResolver func(ctx context.Context, factoryTargetID, workingRoot, requestID string) (factorysessions.SessionStartRequest, error)

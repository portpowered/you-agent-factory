package runtimeopening

import (
	"context"
	"net/http"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// historicalReplayProcessRuntime completes the process lifecycle for an
// inspection-only portable recording. It intentionally starts neither a live
// Factory runtime nor worker sidecars nor an HTTP host.
type historicalReplayProcessRuntime struct{}

func (historicalReplayProcessRuntime) Start(context.Context, context.Context) error { return nil }

func (historicalReplayProcessRuntime) StartWorkers(context.Context) (factorysessions.RuntimeStop, error) {
	return func(context.Context) error { return nil }, nil
}

func (historicalReplayProcessRuntime) RunTransport(context.Context, http.Handler) error { return nil }

func (historicalReplayProcessRuntime) Stop(context.Context) error { return nil }

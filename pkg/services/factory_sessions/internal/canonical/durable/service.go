// Package durable defines the one private durable owner seam consumed by the
// canonical Factory Sessions root.
package durable

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

// Service is the single durable owner capability. Durable implementations
// expose all canonical operations through this seam; the compatibility-shaped
// durable methods remain a separate view and are never called by the root.
type Service interface {
	StartCanonical(context.Context, factorysessions.StartRequest, bool) (durableexecution.CanonicalStartResult, error)
	GetCanonical(context.Context, string) (factorysessions.SessionReadResult, error)
	ListCanonical(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
	ControlCanonical(context.Context, factorysessions.SessionControlRequest) (durableexecution.CanonicalControlResult, error)
	ReadResultCanonical(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error)
	QueryDispatchesCanonical(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error)
	SubscribeResponsesCanonical(context.Context, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error)
}

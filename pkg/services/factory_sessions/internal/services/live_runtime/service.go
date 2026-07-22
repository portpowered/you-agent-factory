package liveruntime

import (
	"context"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// Service owns live Factory Session opening, registry reads, lifecycle control,
// and stop coordination behind the Factory Sessions boundary.
type Service interface {
	OpenForTarget(context.Context, factorysessions.Target) (string, error)
	List(context.Context) ([]factorysessions.ReadProjection, error)
	Get(context.Context, string) (factorysessions.SessionProjection, error)
	Resolve(string) *factorysessions.LiveSession
	Snapshot(context.Context, string) (*factoryruntime.StateSnapshot, error)
	ApplyControl(context.Context, string, factorysessions.LifecycleControlKind, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	Close(context.Context, string) error
}

// Dependencies are the runtime-bound effects used by the live-runtime owner.
// They are supplied by Factory Sessions composition and never selected here.
type Dependencies struct {
	OpenForTarget          func(context.Context, factorysessions.Target) (string, error)
	ListSessionIDs         func() []string
	GetSession             func(string) *factorysessions.LiveSession
	RequireSession         func(string) (*factorysessions.LiveSession, error)
	BuildProjectionContext func(context.Context, *factorysessions.LiveSession) (factorysessions.ProjectionContext, error)
	SessionFactory         func(string) (factoryruntime.Service, error)
	StopSession            func(string) error
	ObserveControl         func(string, factorysessions.LifecycleControlKind, factorysessions.ControlRequest, factorysessions.LifecycleControlOutcome, factorysessions.LifecycleStatus, error)
}

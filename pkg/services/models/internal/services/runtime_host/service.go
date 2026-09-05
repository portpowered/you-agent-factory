// Package runtimehost defines the parent-private Models Runtime Host service.
package runtimehost

import (
	"context"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

// Options supplies explicit host policy and backend effects to Runtime Host.
// Zero values preserve the characterized legacy HTTP host behavior; managed
// LocalAI backends require the pinned protocol and compatibility effects.
type Options struct {
	Platform             models.AssetHostPlatform
	ProtocolNegotiator   modelseffects.HostProtocolNegotiator
	CompatibilityChecker modelseffects.HostCompatibilityChecker
	IdleUnloadAfter      time.Duration
	MaxLoadedRuntimes    int
}

// Service supervises scoped model-host capacity behind the singular Models root.
// Peers reach supervise, health, reuse, and unload behavior only through the
// process-scoped Models service; this interface stays parent-private.
type Service interface {
	InspectModelHost(
		context.Context,
		models.InspectModelHostRequest,
	) (models.InspectModelHostResult, error)
	EnsureModelHost(
		context.Context,
		models.EnsureModelHostRequest,
	) (models.EnsureModelHostResult, error)
	StopModelHost(
		context.Context,
		models.StopModelHostRequest,
	) (models.StopModelHostResult, error)
	AcquireModelLease(
		context.Context,
		models.AcquireModelLeaseRequest,
	) (models.AcquireModelLeaseResult, error)
	GetModelLease(
		context.Context,
		models.GetModelLeaseRequest,
	) (models.GetModelLeaseResult, error)
	ReleaseModelLease(
		context.Context,
		models.ReleaseModelLeaseRequest,
	) (models.ReleaseModelLeaseResult, error)
}

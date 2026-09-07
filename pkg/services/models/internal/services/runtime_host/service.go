// Package runtimehost defines the parent-private Models Runtime Host service.
package runtimehost

import (
	"context"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

type runtimeCorrelationContextKey struct{}

// WithRuntimeCorrelation carries the bounded invocation identity through the
// parent-private host boundary without changing the customer-facing request
// or host contracts.
func WithRuntimeCorrelation(ctx context.Context, correlation string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runtimeCorrelationContextKey{}, correlation)
}

// RuntimeCorrelation returns the invocation identity carried by a host
// operation, if one was supplied by the joined Models flow.
func RuntimeCorrelation(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	correlation, _ := ctx.Value(runtimeCorrelationContextKey{}).(string)
	return correlation
}

// Options supplies explicit host policy and backend effects to Runtime Host.
// Zero values preserve the characterized legacy HTTP host behavior; managed
// LocalAI backends require the pinned protocol and compatibility effects.
type Options struct {
	Platform             models.AssetHostPlatform
	ProtocolNegotiator   modelseffects.HostProtocolNegotiator
	CompatibilityChecker modelseffects.HostCompatibilityChecker
	ResolveSymlinks      modelseffects.HostResolveSymlinks
	RuntimeEvidence      modelseffects.RuntimeEvidenceRecorder
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

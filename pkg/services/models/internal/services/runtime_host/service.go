// Package runtimehost defines the parent-private Models Runtime Host service.
package runtimehost

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

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
}

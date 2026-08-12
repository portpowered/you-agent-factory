// Package contracts exposes exact Factory Sessions construction and effect
// roles to canonical Wire without widening the product-facing root Service.
package contracts

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

type (
	DirectoryInspection                  = roles.DirectoryInspection
	CursorPersistenceFileSystem          = roles.CursorPersistenceFileSystem
	CursorPersistenceTemporaryFile       = roles.CursorPersistenceTemporaryFile
	CursorPersistenceCreateTemporaryFile = roles.CursorPersistenceCreateTemporaryFile
	ExecutionOpeningFileSystem           = roles.ExecutionOpeningFileSystem
	InvocationMetricsRecorder            = roles.InvocationMetricsRecorder
	RequestPreparation                   = roles.RequestPreparation
	RuntimePersistenceFileSystem         = roles.RuntimePersistenceFileSystem
	ModelInvocationOperation             = roles.ModelInvocationOperation
	InvocationOperation                  = roles.InvocationOperation
	InvocationTarget                     = roles.InvocationTarget
	FactoryInvocationOutcome             = roles.FactoryInvocationOutcome
)

// LiveChangeCoordinator is the explicit Factory Sessions construction role
// shared by live and durable execution. It remains outside the product-facing
// service root because it is a composition dependency, not a customer API.
type LiveChangeCoordinator interface {
	ApplyLiveChange(context.Context, string, factorysessions.LiveChangeRequest, factorysessions.LiveChangeOperation) (factorysessions.LiveChangeResult, error)
	RecoverLiveChange(context.Context, string, string, factorysessions.LiveChangeOperation) (factorysessions.LiveChangeResult, error)
}

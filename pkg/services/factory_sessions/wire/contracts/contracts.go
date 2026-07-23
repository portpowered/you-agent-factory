// Package contracts exposes exact Factory Sessions construction and effect
// roles to canonical Wire without widening the product-facing root Service.
package contracts

import "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"

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

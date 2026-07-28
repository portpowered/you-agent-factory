// Package envdiagnostics is a transitional compile shim that re-exports command
// environment diagnostic projection from the private workstations destination.
package envdiagnostics

import (
	workstationenv "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/envdiagnostics"
)

const (
	RedactedCommandEnvValue         = workstationenv.RedactedCommandEnvValue
	MetadataOnlyCommandEnvValue     = workstationenv.MetadataOnlyCommandEnvValue
	CommandEnvClassificationSafe    = workstationenv.CommandEnvClassificationSafe
	CommandEnvClassificationRedacted = workstationenv.CommandEnvClassificationRedacted
	CommandEnvClassificationMetadataOnly = workstationenv.CommandEnvClassificationMetadataOnly
)

type (
	CommandEnvClassification       = workstationenv.CommandEnvClassification
	CommandEnvDiagnosticProjection = workstationenv.CommandEnvDiagnosticProjection
)

var (
	ClassifyCommandEnvKey            = workstationenv.ClassifyCommandEnvKey
	ProjectCommandEnvForDiagnostics  = workstationenv.ProjectCommandEnvForDiagnostics
	CommandEnvDiagnosticMetadata     = workstationenv.CommandEnvDiagnosticMetadata
)

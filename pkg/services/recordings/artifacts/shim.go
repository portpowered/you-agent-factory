package artifacts

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	artifactsimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/artifacts"
)

const (
	KindJavaScriptFactorySession = artifactsimpl.KindJavaScriptFactorySession
	CurrentSchemaVersion         = artifactsimpl.CurrentSchemaVersion
	ReplayCompatibilityVersion   = artifactsimpl.ReplayCompatibilityVersion
	MaxSecretsRedacted           = artifactsimpl.MaxSecretsRedacted
)

const (
	CodeMalformedContract  = artifactsimpl.CodeMalformedContract
	CodeUnsupportedVersion = artifactsimpl.CodeUnsupportedVersion
	CodeInvalidIdentity    = artifactsimpl.CodeInvalidIdentity
	CodeInvalidDigest      = artifactsimpl.CodeInvalidDigest
	CodeInvalidSummary     = artifactsimpl.CodeInvalidSummary
)

var (
	Build                     = artifactsimpl.Build
	DecodeAndValidate         = artifactsimpl.DecodeAndValidate
	Validate                  = artifactsimpl.Validate
	ApplyJavaScriptProjectionFacts = artifactsimpl.ApplyJavaScriptProjectionFacts
	NewAtomicWriter           = artifactsimpl.NewAtomicWriter
)

type (
	Recording          = artifactsimpl.Recording
	SessionSummary     = artifactsimpl.SessionSummary
	SourceSummary      = artifactsimpl.SourceSummary
	ArtifactSummary    = artifactsimpl.ArtifactSummary
	EventSummary       = artifactsimpl.EventSummary
	CheckpointSummary  = artifactsimpl.CheckpointSummary
	ResultProjection   = artifactsimpl.ResultProjection
	FailureSummary     = artifactsimpl.FailureSummary
	AvailabilityDetail = artifactsimpl.AvailabilityDetail
	RedactionMetadata  = artifactsimpl.RedactionMetadata

	DiagnosticCode = artifactsimpl.DiagnosticCode
	Diagnostic     = artifactsimpl.Diagnostic

	CanonicalFacts      = artifactsimpl.CanonicalFacts
	CanonicalCheckpoint = artifactsimpl.CanonicalCheckpoint
	CanonicalArtifact   = artifactsimpl.CanonicalArtifact
	CanonicalResult     = artifactsimpl.CanonicalResult

	TemporaryFile       = artifactsimpl.TemporaryFile
	MakeDirectories     = artifactsimpl.MakeDirectories
	CreateTemporaryFile = artifactsimpl.CreateTemporaryFile
	RemovePath          = artifactsimpl.RemovePath
	RenamePath          = artifactsimpl.RenamePath
	Writer              = artifactsimpl.Writer
	AtomicWriter        = artifactsimpl.AtomicWriter
)

// Preserve factory_definitions import for vocabulary boundary tests that assert
// the shim still depends on the published Factory contract.
var _ = interfaces.FactorySessionJavaScriptRuntimeState{}

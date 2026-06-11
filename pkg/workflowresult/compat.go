// Deprecated: use github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result instead.
// This package is a Batch 001 compatibility shim; core runtime and API code must import the orchestrator-owned path directly.
package workflowresult

import target "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"

type (
	Issue              = target.Issue
	Result             = target.Result
	TypedValue         = target.TypedValue
	SessionResultInput = target.SessionResultInput
	ParsedArtifactURI  = target.ParsedArtifactURI
)

const (
	CodeNonJSONValue               = target.CodeNonJSONValue
	CodeUnsupportedType            = target.CodeUnsupportedType
	CodeUnresolvedPromise          = target.CodeUnresolvedPromise
	CodeCyclicValue                = target.CodeCyclicValue
	CodeHostHandle                 = target.CodeHostHandle
	CodeUnsupportedBinary          = target.CodeUnsupportedBinary
	CodeArtifactURIMalformed       = target.CodeArtifactURIMalformed
	CodeArtifactURIInvalidID       = target.CodeArtifactURIInvalidID
	CodeArtifactURIPathTraversal   = target.CodeArtifactURIPathTraversal
	CodeArtifactURIHostPath        = target.CodeArtifactURIHostPath
	CodeArtifactURISessionMismatch = target.CodeArtifactURISessionMismatch
	ArtifactURIScheme              = target.ArtifactURIScheme
	DefaultMaxEmbeddedBytes        = target.DefaultMaxEmbeddedBytes
)

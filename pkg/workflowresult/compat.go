// Package workflowresult is a Batch 001 compatibility shim for the legacy root
// workflow result import path.
//
// Deprecated: canonical ownership for JavaScript workflow result lives in
// github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result. Core
// runtime and API code must import pkg/orchestrators/javascript/result directly.
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

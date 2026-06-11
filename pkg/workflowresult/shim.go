// Package workflowresult is a transitional compatibility shim for JavaScript
// orchestrator result validation and projection.
//
// Deprecated: use pkg/orchestrators/javascript/result. This shim delegates to
// orchestrator ownership and is not the final package boundary.
package workflowresult

import jsresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"

type (
	Issue             = jsresult.Issue
	ParsedArtifactURI = jsresult.ParsedArtifactURI
	Result            = jsresult.Result
	SessionResultInput = jsresult.SessionResultInput
	TypedValue        = jsresult.TypedValue
)

const (
	ArtifactURIScheme              = jsresult.ArtifactURIScheme
	CodeArtifactURISessionMismatch = jsresult.CodeArtifactURISessionMismatch
	CodeCyclicValue                = jsresult.CodeCyclicValue
	CodeHostHandle                 = jsresult.CodeHostHandle
	CodeNonJSONValue               = jsresult.CodeNonJSONValue
	CodeUnresolvedPromise          = jsresult.CodeUnresolvedPromise
	CodeUnsupportedBinary          = jsresult.CodeUnsupportedBinary
	CodeUnsupportedType            = jsresult.CodeUnsupportedType
	DefaultMaxEmbeddedBytes        = jsresult.DefaultMaxEmbeddedBytes
)

var (
	BuildLiveSessionResult          = jsresult.BuildLiveSessionResult
	BuildSessionResult              = jsresult.BuildSessionResult
	BuildSessionResultUpdatedPayload = jsresult.BuildSessionResultUpdatedPayload
	FormatArtifactURI               = jsresult.FormatArtifactURI
	ParseArtifactURI                = jsresult.ParseArtifactURI
	ProjectPrimaryResult            = jsresult.ProjectPrimaryResult
	ValidateArtifactURIForSession   = jsresult.ValidateArtifactURIForSession
	ValidateTypedValue              = jsresult.ValidateTypedValue
)

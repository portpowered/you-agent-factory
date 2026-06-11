// Deprecated: use github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result instead.
// This package is a Batch 001 compatibility shim; core runtime and API code must import the orchestrator-owned path directly.
package workflowresult

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"

	target "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

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

func ProjectPrimaryResult(sessionID string, value TypedValue, artifacts []interfaces.FactorySessionArtifactState) ([]interfaces.WorkContentPart, Result) {
	return target.ProjectPrimaryResult(sessionID, value, artifacts)
}

func BuildLiveSessionResult(input SessionResultInput) factoryapi.FactorySessionLiveResult {
	return target.BuildLiveSessionResult(input)
}

func BuildSessionResult(input SessionResultInput) factoryapi.FactorySessionResult {
	return target.BuildSessionResult(input)
}

func BuildSessionResultUpdatedPayload(input SessionResultInput) factoryapi.SessionResultUpdatedEventPayload {
	return target.BuildSessionResultUpdatedPayload(input)
}

func ValidateTypedValue(value TypedValue) Result { return target.ValidateTypedValue(value) }

func FormatArtifactURI(sessionID string, artifactID string) string {
	return target.FormatArtifactURI(sessionID, artifactID)
}

func ParseArtifactURI(raw string) (ParsedArtifactURI, []Issue) { return target.ParseArtifactURI(raw) }

func ValidateArtifactURIForSession(raw string, expectedSessionID string) []Issue {
	return target.ValidateArtifactURIForSession(raw, expectedSessionID)
}

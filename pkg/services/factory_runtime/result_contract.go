package factory

import runtimecontract "github.com/portpowered/infinite-you/pkg/services/factory_runtime/runtimecontract"

type (
	Issue             = runtimecontract.ResultIssue
	Result            = runtimecontract.ResultValidation
	TypedValue        = runtimecontract.TypedValue
	ParsedArtifactURI = runtimecontract.ParsedArtifactURI
)

const (
	CodeNonJSONValue               = runtimecontract.CodeNonJSONValue
	CodeUnsupportedType            = runtimecontract.CodeUnsupportedType
	CodeUnresolvedPromise          = runtimecontract.CodeUnresolvedPromise
	CodeCyclicValue                = runtimecontract.CodeCyclicValue
	CodeHostHandle                 = runtimecontract.CodeHostHandle
	CodeUnsupportedBinary          = runtimecontract.CodeUnsupportedBinary
	CodeArtifactURIMalformed       = runtimecontract.CodeArtifactURIMalformed
	CodeArtifactURIInvalidID       = runtimecontract.CodeArtifactURIInvalidID
	CodeArtifactURIPathTraversal   = runtimecontract.CodeArtifactURIPathTraversal
	CodeArtifactURIHostPath        = runtimecontract.CodeArtifactURIHostPath
	CodeArtifactURISessionMismatch = runtimecontract.CodeArtifactURISessionMismatch
)

var (
	FormatArtifactURI             = runtimecontract.FormatArtifactURI
	ParseArtifactURI              = runtimecontract.ParseArtifactURI
	ValidateArtifactURIForSession = runtimecontract.ValidateArtifactURIForSession
	ValidateTypedValue            = runtimecontract.ValidateTypedValue
)

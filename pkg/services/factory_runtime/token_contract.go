package factory

import "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"

// Runtime token contracts are published at the Factory Runtime service root.
// The token subpackage remains an owner-internal implementation surface.
type (
	RuntimeTokenDataType = token.DataType
	RuntimeTokenColor    = token.Color
	RuntimeToken         = token.Token
	RuntimeTokenHistory  = token.History
	RuntimeTokenFailure  = token.Failure
)

const (
	RuntimeTokenDataTypeResource = token.DataTypeResource
	RuntimeTokenDataTypeWork     = token.DataTypeWork
)

var CloneRuntimeToken = token.Clone
var ClearRuntimeTokenGuardBlockingFields = token.ClearGuardBlockingFields

// Package namevalue exposes transitional compile-time re-exports of the
// validation-owned authored schema helper under internal/services/validation.
// Production ownership lives in internal/services/validation/authoredmodel/namevalue.
package namevalue

import namevalueimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/namevalue"

const TypeLocalizableAsset = namevalueimpl.TypeLocalizableAsset

type (
	Config          = namevalueimpl.Config
	ValidationError = namevalueimpl.ValidationError
)

var (
	Validate = namevalueimpl.Validate
	Resolve  = namevalueimpl.Resolve
)

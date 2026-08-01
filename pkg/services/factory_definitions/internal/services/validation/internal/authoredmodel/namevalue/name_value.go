package namevalue

import factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"

const TypeLocalizableAsset = factorycontracts.NameValueTypeLocalizableAsset

type Config = factorycontracts.NameValueConfig
type ValidationError = factorycontracts.NameValueValidationError

func Validate(value Config) error { return factorycontracts.ValidateNameValue(value) }

func Resolve(value Config, locale string) string {
	return factorycontracts.ResolveNameValue(value, locale)
}

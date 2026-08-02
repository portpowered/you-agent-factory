package http

import factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

func decodeDetailsParams(
	params factoryapi.GetProviderSessionDetailsParams,
) (provider string, kind string, id string) {
	// Keep the identifier byte-for-byte identical to the generated request. The
	// Provider Sessions root owns the established validation and lookup
	// semantics, including whitespace and blank-identifier errors.
	return string(params.Provider), string(params.Kind), params.Id
}

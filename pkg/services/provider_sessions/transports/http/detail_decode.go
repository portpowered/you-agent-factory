package http

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type requestValidationError struct {
	message string
}

func (e requestValidationError) Error() string {
	return e.message
}

func decodeDetailsParams(
	params factoryapi.GetProviderSessionDetailsParams,
) (provider string, kind string, id string, err error) {
	id = strings.TrimSpace(params.Id)
	if id == "" {
		return "", "", "", requestValidationError{message: "provider session id is required"}
	}
	return string(params.Provider), string(params.Kind), id, nil
}

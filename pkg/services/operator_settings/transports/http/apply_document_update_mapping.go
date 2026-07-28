package http

import (
	"errors"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

var (
	// ErrInvalidUpdatePath reports a missing or blank document path at the
	// Operator Settings update HTTP adapter edge.
	ErrInvalidUpdatePath = errors.New("operator settings http: invalid update path")
)

// ApplyDocumentUpdateInput carries decoded HTTP inputs for one Operator
// Settings document-update operation owned by this adapter.
type ApplyDocumentUpdateInput struct {
	Path                 string
	ExpectedBackendScope string
	Provider             *string
	Model                *string
}

// ApplyDocumentUpdateResponse is the adapter-owned HTTP success shape for one
// document-update outcome.
type ApplyDocumentUpdateResponse struct {
	Path      string                  `json:"path"`
	Persisted bool                    `json:"persisted"`
	Document  factoryapi.GlobalConfig `json:"document"`
}

// IsApplyDocumentUpdateBadRequest reports whether an error is a decode/validation
// failure that maps to a typed bad-request HTTP outcome before root invocation.
func IsApplyDocumentUpdateBadRequest(err error) bool {
	return errors.Is(err, ErrInvalidUpdatePath)
}

// ApplyDocumentUpdateRequestFromHTTP maps one update-document HTTP request into
// the accepted Operator Settings root request vocabulary.
func ApplyDocumentUpdateRequestFromHTTP(
	input ApplyDocumentUpdateInput,
) (operatorsettings.ApplyDocumentUpdateRequest, error) {
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return operatorsettings.ApplyDocumentUpdateRequest{}, ErrInvalidUpdatePath
	}
	request := operatorsettings.ApplyDocumentUpdateRequest{
		Path:                 path,
		ExpectedBackendScope: strings.TrimSpace(input.ExpectedBackendScope),
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Provider: optionalStringPointerValue(input.Provider),
			Model:    optionalStringPointerValue(input.Model),
		},
	}
	if err := request.Validate(); err != nil {
		return operatorsettings.ApplyDocumentUpdateRequest{}, err
	}
	return request, nil
}

// ApplyDocumentUpdateResponseToHTTP encodes one fake-root update-document result
// into the adapter-owned HTTP success response shape.
func ApplyDocumentUpdateResponseToHTTP(
	result operatorsettings.ApplyDocumentUpdateResult,
) ApplyDocumentUpdateResponse {
	return ApplyDocumentUpdateResponse{
		Path:      strings.TrimSpace(result.Path),
		Persisted: result.Persisted,
		Document:  documentToGlobalConfig(result.Document),
	}
}

func optionalStringPointerValue(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

package http

import "context"

// ApplyDocumentUpdate decodes one update-document HTTP request, invokes the
// accepted Operator Settings root, and encodes the success response shape.
func (a *Adapter) ApplyDocumentUpdate(
	ctx context.Context,
	input ApplyDocumentUpdateInput,
) (ApplyDocumentUpdateResponse, error) {
	request, err := ApplyDocumentUpdateRequestFromHTTP(input)
	if err != nil {
		return ApplyDocumentUpdateResponse{}, err
	}
	if err := guardRequestContext(ctx); err != nil {
		return ApplyDocumentUpdateResponse{}, err
	}
	result, err := a.invokeApplyDocumentUpdate(ctx, request)
	if err != nil {
		return ApplyDocumentUpdateResponse{}, err
	}
	return ApplyDocumentUpdateResponseToHTTP(result), nil
}

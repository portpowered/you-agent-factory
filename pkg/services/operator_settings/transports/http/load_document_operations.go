package http

import "context"

// LoadDocument decodes one load-document HTTP request, invokes the accepted
// Operator Settings root, and encodes the success response shape.
func (a *Adapter) LoadDocument(
	ctx context.Context,
	input LoadDocumentInput,
) (LoadDocumentResponse, error) {
	request, err := LoadDocumentRequestFromHTTP(input)
	if err != nil {
		return LoadDocumentResponse{}, err
	}
	if err := guardRequestContext(ctx); err != nil {
		return LoadDocumentResponse{}, err
	}
	result, err := a.invokeLoadDocument(ctx, request)
	if err != nil {
		return LoadDocumentResponse{}, err
	}
	return LoadDocumentResponseToHTTP(result), nil
}

package http

// ApplyDocumentUpdate decodes one update-document HTTP request, invokes the
// accepted Operator Settings root, and encodes the success response shape.
func (a *Adapter) ApplyDocumentUpdate(
	input ApplyDocumentUpdateInput,
) (ApplyDocumentUpdateResponse, error) {
	request, err := ApplyDocumentUpdateRequestFromHTTP(input)
	if err != nil {
		return ApplyDocumentUpdateResponse{}, err
	}
	result, err := a.invokeApplyDocumentUpdate(request)
	if err != nil {
		return ApplyDocumentUpdateResponse{}, err
	}
	return ApplyDocumentUpdateResponseToHTTP(result), nil
}

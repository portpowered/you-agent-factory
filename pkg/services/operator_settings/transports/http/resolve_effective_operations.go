package http

// ResolveEffective decodes one effective-resolution HTTP request, invokes the
// accepted Operator Settings root, and encodes the success response shape.
func (a *Adapter) ResolveEffective(
	input ResolveEffectiveInput,
) (ResolveEffectiveResponse, error) {
	request, err := ResolveEffectiveRequestFromHTTP(input)
	if err != nil {
		return ResolveEffectiveResponse{}, err
	}
	result, err := a.invokeResolveEffective(request)
	if err != nil {
		return ResolveEffectiveResponse{}, err
	}
	return ResolveEffectiveResponseToHTTP(result), nil
}

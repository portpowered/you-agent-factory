package models

// InvocationProtocolRequest carries the ordered, codec-normalized values for
// one generic protocol call. Slot order and detected media types are part of
// the request contract so adapters do not need to reconstruct CLI intent.
type InvocationProtocolRequest struct {
	Operation  string
	Prompt     string
	Inputs     []InvocationProtocolInput
	Parameters []OperationParameter
}

// InvocationProtocolInput is one provider-neutral protocol value.
type InvocationProtocolInput struct {
	Slot      string
	Modality  Modality
	MediaType string
	Content   string
	Reference string
}

// InvocationProtocolResponse is the detached text response returned by the
// generic protocol adapter.
type InvocationProtocolResponse struct {
	Text string
}

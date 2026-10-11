package agentmessages

// Address contains no authority or Factory scope. Legacy resolution receives
// the separately selected Factory Session at the operation boundary.
type Address struct {
	WorkerSessionID string `json:"workerSessionId"`
}

// SendRequest is shared by send and reply. An omitted To on a reply addresses
// the original sender; only the server determines sender identity and hop.
type SendRequest struct {
	RequestID        string      `json:"requestId"`
	To               *Address    `json:"to,omitempty"`
	Body             string      `json:"body"`
	BodySecret       bool        `json:"bodySecret,omitempty"`
	InReplyTo        string      `json:"inReplyTo,omitempty"`
	Delivery         string      `json:"delivery,omitempty"`
	IfEnded          string      `json:"ifEnded,omitempty"`
	ReplyIfEnded     string      `json:"replyIfEnded,omitempty"`
	Correlation      Correlation `json:"correlation,omitempty"`
	ExpiresInSeconds *int        `json:"expiresInSeconds,omitempty"`
}

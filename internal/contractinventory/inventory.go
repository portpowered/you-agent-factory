package contractinventory

// FormatVersion is the inventory schema version written to formatVersion.
const FormatVersion = "rest-operations/v1"

// Inventory is the canonical REST operation identity document.
type Inventory struct {
	FormatVersion string      `json:"formatVersion"`
	Operations    []Operation `json:"operations"`
}

// Operation records one REST operation identity from an OpenAPI document.
type Operation struct {
	OperationID       string     `json:"operationId"`
	Method            string     `json:"method"`
	Path              string     `json:"path"`
	XDocID            string     `json:"xDocId,omitempty"`
	HasSummary        bool       `json:"hasSummary"`
	HasDescription    bool       `json:"hasDescription"`
	RequestMediaTypes []string   `json:"requestMediaTypes"`
	Responses         []Response `json:"responses"`
}

// Response records one declared response and its media types.
type Response struct {
	Status     string   `json:"status"`
	MediaTypes []string `json:"mediaTypes"`
}

package contractinventory

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

var httpMethodOrder = map[string]int{
	"connect": 0,
	"delete":  1,
	"get":     2,
	"head":    3,
	"options": 4,
	"patch":   5,
	"post":    6,
	"put":     7,
	"trace":   8,
}

// ExtractFromOpenAPIYAML parses bundled OpenAPI YAML and returns a sorted inventory.
func ExtractFromOpenAPIYAML(data []byte) (*Inventory, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("load openapi document: %w", err)
	}
	return ExtractFromDocument(doc)
}

// ExtractFromDocument builds a sorted inventory from a loaded OpenAPI document.
func ExtractFromDocument(doc *openapi3.T) (*Inventory, error) {
	if doc == nil {
		return nil, fmt.Errorf("openapi document is nil")
	}

	var operations []Operation
	if doc.Paths != nil {
		for path, pathItem := range doc.Paths.Map() {
			if pathItem == nil {
				continue
			}
			for method, operation := range pathItem.Operations() {
				if operation == nil {
					continue
				}
				operations = append(operations, buildOperation(path, method, operation))
			}
		}
	}

	sortOperations(operations)

	return &Inventory{
		FormatVersion: FormatVersion,
		Operations:    operations,
	}, nil
}

func buildOperation(path, method string, operation *openapi3.Operation) Operation {
	record := Operation{
		OperationID:       operation.OperationID,
		Method:            strings.ToUpper(method),
		Path:              path,
		XDocID:            extensionString(operation.Extensions, "x-doc-id"),
		HasSummary:        strings.TrimSpace(operation.Summary) != "",
		HasDescription:    strings.TrimSpace(operation.Description) != "",
		RequestMediaTypes: mediaTypesFromRequestBody(operation.RequestBody),
		Responses:         responsesFromOperation(operation.Responses),
	}
	record.RequestMediaTypes = sortedCopy(record.RequestMediaTypes)
	for i := range record.Responses {
		record.Responses[i].MediaTypes = sortedCopy(record.Responses[i].MediaTypes)
	}
	sortResponses(record.Responses)
	return record
}

func responsesFromOperation(responses *openapi3.Responses) []Response {
	if responses == nil {
		return []Response{}
	}

	records := make([]Response, 0, responses.Len())
	for status, responseRef := range responses.Map() {
		records = append(records, Response{
			Status:     status,
			MediaTypes: mediaTypesFromResponseRef(responseRef),
		})
	}
	if len(records) == 0 {
		return []Response{}
	}
	return records
}

func mediaTypesFromRequestBody(requestBody *openapi3.RequestBodyRef) []string {
	if requestBody == nil || requestBody.Value == nil || requestBody.Value.Content == nil {
		return []string{}
	}
	return mediaTypesFromContent(requestBody.Value.Content)
}

func mediaTypesFromResponseRef(responseRef *openapi3.ResponseRef) []string {
	if responseRef == nil || responseRef.Value == nil || responseRef.Value.Content == nil {
		return []string{}
	}
	return mediaTypesFromContent(responseRef.Value.Content)
}

func mediaTypesFromContent(content openapi3.Content) []string {
	if len(content) == 0 {
		return []string{}
	}
	mediaTypes := make([]string, 0, len(content))
	for mediaType := range content {
		mediaTypes = append(mediaTypes, mediaType)
	}
	return mediaTypes
}

func extensionString(extensions map[string]any, key string) string {
	if extensions == nil {
		return ""
	}
	value, ok := extensions[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func sortOperations(operations []Operation) {
	slices.SortFunc(operations, func(left, right Operation) int {
		if cmp := strings.Compare(left.Path, right.Path); cmp != 0 {
			return cmp
		}
		if cmp := compareHTTPMethod(left.Method, right.Method); cmp != 0 {
			return cmp
		}
		return strings.Compare(left.OperationID, right.OperationID)
	})
}

func sortResponses(responses []Response) {
	slices.SortFunc(responses, func(left, right Response) int {
		return compareResponseStatus(left.Status, right.Status)
	})
}

func compareHTTPMethod(left, right string) int {
	leftOrder, leftKnown := httpMethodOrder[strings.ToLower(left)]
	rightOrder, rightKnown := httpMethodOrder[strings.ToLower(right)]
	switch {
	case leftKnown && rightKnown && leftOrder != rightOrder:
		return leftOrder - rightOrder
	case leftKnown != rightKnown:
		if leftKnown {
			return -1
		}
		return 1
	default:
		return strings.Compare(left, right)
	}
}

func compareResponseStatus(left, right string) int {
	if left == "default" && right != "default" {
		return 1
	}
	if right == "default" && left != "default" {
		return -1
	}
	return strings.Compare(left, right)
}

func sortedCopy(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	copied := slices.Clone(values)
	slices.Sort(copied)
	return copied
}

// ValidateLoadedDocument validates doc when callers need loader diagnostics.
func ValidateLoadedDocument(doc *openapi3.T) error {
	if doc == nil {
		return fmt.Errorf("openapi document is nil")
	}
	return doc.Validate(context.Background())
}

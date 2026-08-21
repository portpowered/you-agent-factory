package apicontract_test

import (
	"testing"

	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

func TestOpenAPIContract_CostsReportIsTypedAndAmountsAreOptionalStrings(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	paths := objectField(t, doc, "paths")
	operation := pathOperation(t, paths, "/metrics/costs", "get")
	if operation["operationId"] != "getMetricsCosts" {
		t.Fatalf("costs operationId = %v, want getMetricsCosts", operation["operationId"])
	}
	parameters, ok := operation["parameters"].([]any)
	if !ok || len(parameters) != 1 {
		t.Fatalf("costs parameters = %#v, want one optional session_id parameter", operation["parameters"])
	}
	parameter, ok := parameters[0].(map[string]any)
	if !ok || parameter["name"] != "session_id" || parameter["in"] != "query" || parameter["required"] != false {
		t.Fatalf("costs session parameter = %#v", parameters[0])
	}
	assertResponseRef(t, operation, "400", "#/components/responses/BadRequest")
	assertResponseRef(t, operation, "500", "#/components/responses/InternalError")

	schemas := componentSchemas(t, doc)
	report := schemaObject(t, schemas, "CostsReport")
	assertRequiredFields(t, report, "scope", "currency", "status", "coverage", "line_items", "work_items", "worker_sessions", "provider_models", "factory_sessions")
	reportProperties := schemaProperties(t, report, "CostsReport")
	pricedSubtotal, ok := reportProperties["priced_subtotal"].(map[string]any)
	if !ok || pricedSubtotal["type"] != "string" {
		t.Fatalf("CostsReport.priced_subtotal = %#v, want optional string", reportProperties["priced_subtotal"])
	}
	lineItem := schemaObject(t, schemas, "CostsLineItem")
	lineProperties := schemaProperties(t, lineItem, "CostsLineItem")
	for _, field := range []string{"input_tokens", "output_tokens", "cached_input_tokens", "reasoning_output_tokens"} {
		property, ok := lineProperties[field].(map[string]any)
		if !ok || property["type"] != "integer" {
			t.Fatalf("CostsLineItem.%s = %#v, want integer", field, lineProperties[field])
		}
	}
	status, ok := reportProperties["status"].(map[string]any)
	if !ok {
		t.Fatalf("CostsReport.status = %#v", reportProperties["status"])
	}
	assertEnumValues(t, status, "CostsReport.status", []string{"PRICED", "PARTIAL", "UNPRICED", "NO_USAGE"})
}

func TestGeneratedGoClientBuildsMetricsCostsRequestAndExposesTypedResponses(t *testing.T) {
	sessionID := "session one"
	request, err := generatedclient.NewGetMetricsCostsRequest(
		"http://localhost:7437",
		&generatedclient.GetMetricsCostsParams{SessionId: &sessionID},
	)
	if err != nil {
		t.Fatalf("build generated costs request: %v", err)
	}
	if got, want := request.URL.EscapedPath(), "/metrics/costs"; got != want {
		t.Fatalf("generated costs path = %q, want %q", got, want)
	}
	if got := request.URL.Query().Get("session_id"); got != sessionID {
		t.Fatalf("generated costs query session_id = %q, want %q", got, sessionID)
	}

	response := generatedclient.GetMetricsCostsClientResponse{
		JSON200: &generatedclient.CostsReport{},
		JSON400: &generatedclient.BadRequest{},
		JSON500: &generatedclient.InternalError{},
	}
	if response.JSON200 == nil || response.JSON400 == nil || response.JSON500 == nil {
		t.Fatal("generated costs client must expose success and typed error responses")
	}
}

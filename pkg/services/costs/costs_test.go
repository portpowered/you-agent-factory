package costs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestQueryNilOperationAndRequestValidation(t *testing.T) {
	t.Parallel()

	_, err := CostsQuery(nil).Query(context.Background(), QueryRequest{})
	var queryErr *QueryError
	if !errors.As(err, &queryErr) || queryErr.Kind != QueryErrorInvalidInput {
		t.Fatalf("nil query error = %v, want invalid input", err)
	}
	if err := (QueryRequest{}).Validate(); err == nil {
		t.Fatal("empty QueryRequest.Validate() = nil, want error")
	}
	if err := (QueryRequest{MetricsRoot: "metrics"}).Validate(); err != nil {
		t.Fatalf("missing compatibility settings path = %v, want no validation error", err)
	}
	operation := CostsQuery(func(context.Context, QueryRequest) (Report, error) {
		return Report{Status: StatusNoUsage}, nil
	})
	result, err := operation.QueryCosts(context.Background(), QueryRequest{})
	if err != nil || result.Status != StatusNoUsage {
		t.Fatalf("QueryCosts() = %#v, error = %v, want delegated operation", result, err)
	}
}

func TestQueryErrorUnwrapAndAliases(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause")
	err := &QueryError{Kind: QueryErrorMetricsFailed, Message: "metrics failed", Cause: cause}
	if !errors.Is(err, cause) || err.Error() != "metrics failed" {
		t.Fatalf("QueryError = %v, want wrapped cause and message", err)
	}
	var query Query
	if query != nil {
		t.Fatal("Query alias zero value is non-nil")
	}
	var nilError *QueryError
	if nilError.Error() != "" || nilError.Unwrap() != nil {
		t.Fatalf("nil QueryError methods = %q/%v, want empty/nil", nilError.Error(), nilError.Unwrap())
	}
}

func TestPartialReportJSONRetainsKnownCostAndUnknownCoverage(t *testing.T) {
	t.Parallel()

	knownCost := "1.25"
	input, output, total := int64(100), int64(25), int64(125)
	provider, model := "CODEX", "unknown"
	report := Report{
		Status:                StatusPartial,
		KnownCost:             &knownCost,
		TokenTotals:           TokenTotals{TotalTokens: &total, InputTokens: &input, OutputTokens: &output},
		UnpricedDispatchCount: 1,
		UnpricedPairs:         []UnpricedPair{{Provider: &provider, Model: &model, DispatchCount: 1}},
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal partial report: %v", err)
	}
	var shape map[string]any
	if err := json.Unmarshal(encoded, &shape); err != nil {
		t.Fatalf("decode partial report: %v", err)
	}
	if shape["status"] != string(StatusPartial) || shape["known_cost"] != knownCost {
		t.Fatalf("partial JSON = %s, want PARTIAL and known cost", encoded)
	}
	if shape["unpriced_dispatch_count"] != float64(1) || shape["unpriced_pairs"] == nil || shape["token_totals"] == nil {
		t.Fatalf("partial JSON = %s, want distinct unknown and token facts", encoded)
	}
}

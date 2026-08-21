package costs

import (
	"context"
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
	if err := (QueryRequest{MetricsRoot: "metrics"}).Validate(); err == nil {
		t.Fatal("missing settings path validation = nil, want error")
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

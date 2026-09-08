package platform_conformance

import "fmt"

// SchemaError identifies malformed persisted or declarative conformance data.
// The stable code and field let callers classify a failure without parsing its
// human-readable message.
type SchemaError struct {
	Code     string
	Field    string
	Expected string
	Observed string
	Cause    error
}

func (err *SchemaError) Error() string {
	if err == nil {
		return ""
	}
	message := fmt.Sprintf("platform conformance schema error: code=%s field=%s", err.Code, err.Field)
	if err.Expected != "" {
		message += fmt.Sprintf(" expected=%s", err.Expected)
	}
	if err.Observed != "" {
		message += fmt.Sprintf(" observed=%s", err.Observed)
	}
	return message
}

func (err *SchemaError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// AdmissionError identifies a fail-closed pre-launch rejection.
type AdmissionError struct {
	Code     string
	Field    string
	Expected string
	Observed string
	Cause    error
}

func (err *AdmissionError) Error() string {
	if err == nil {
		return ""
	}
	message := fmt.Sprintf("platform conformance admission failed: code=%s field=%s", err.Code, err.Field)
	if err.Expected != "" {
		message += fmt.Sprintf(" expected=%s", err.Expected)
	}
	if err.Observed != "" {
		message += fmt.Sprintf(" observed=%s", err.Observed)
	}
	return message
}

func (err *AdmissionError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// BudgetError identifies a durable-ledger rejection. It is deliberately
// separate from AdmissionError because callers may want to distinguish a
// bad run declaration from a ledger that was already consumed or finalized.
type BudgetError struct {
	Code     string
	Field    string
	Expected string
	Observed string
	Cause    error
}

func (err *BudgetError) Error() string {
	if err == nil {
		return ""
	}
	message := fmt.Sprintf("platform conformance budget failed: code=%s field=%s", err.Code, err.Field)
	if err.Expected != "" {
		message += fmt.Sprintf(" expected=%s", err.Expected)
	}
	if err.Observed != "" {
		message += fmt.Sprintf(" observed=%s", err.Observed)
	}
	return message
}

func (err *BudgetError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func schemaFailure(code, field, expected, observed string) *SchemaError {
	return &SchemaError{Code: code, Field: field, Expected: expected, Observed: observed}
}

func admissionFailure(code, field, expected, observed string, cause error) *AdmissionError {
	return &AdmissionError{
		Code: code, Field: field, Expected: expected, Observed: observed, Cause: cause,
	}
}

func budgetFailure(code, field, expected, observed string, cause error) *BudgetError {
	return &BudgetError{
		Code: code, Field: field, Expected: expected, Observed: observed, Cause: cause,
	}
}

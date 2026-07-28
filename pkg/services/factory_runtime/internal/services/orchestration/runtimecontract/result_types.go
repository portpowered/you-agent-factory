package runtimecontract

import "encoding/json"

// ResultIssue is one JavaScript result validation diagnostic.
type ResultIssue struct {
	Code    string
	Message string
	Path    string
}

// ResultValidation aggregates JavaScript result validation issues.
type ResultValidation struct {
	Issues []ResultIssue
}

// HasIssues reports whether validation found one or more issues.
func (r ResultValidation) HasIssues() bool {
	return len(r.Issues) > 0
}

// TypedValue carries one JavaScript return/final value at the runtime
// contract boundary.
type TypedValue struct {
	JSON       json.RawMessage
	Unresolved bool
	Function   bool
	HostHandle string
	RawBinary  []byte
	Visited    map[uintptr]struct{}
	HostObject any
}

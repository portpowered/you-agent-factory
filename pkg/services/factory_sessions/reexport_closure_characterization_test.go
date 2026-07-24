package factorysessions_test

import (
	"reflect"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// TestRootOwnsPublishedExecutionAndEffectContracts proves story 008: peer-facing
// durable-execution and effect-port vocabulary is owned by the Sessions root
// package, not re-exported nested internal/execution or internal/contracts types.
func TestRootOwnsPublishedExecutionAndEffectContracts(t *testing.T) {
	t.Parallel()

	const rootPkg = "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

	cases := []struct {
		name string
		typ  reflect.Type
	}{
		{name: "StartRequest", typ: reflect.TypeOf(factorysessions.StartRequest{})},
		{name: "AsyncStartResult", typ: reflect.TypeOf(factorysessions.AsyncStartResult{})},
		{name: "ControlError", typ: reflect.TypeOf(factorysessions.ControlError{})},
		{name: "ResumeError", typ: reflect.TypeOf(factorysessions.ResumeError{})},
		{name: "ExecutionValidationError", typ: reflect.TypeOf(factorysessions.ExecutionValidationError{})},
		{name: "SessionReadResult", typ: reflect.TypeOf(factorysessions.SessionReadResult{})},
		{name: "ControlRequest", typ: reflect.TypeOf(factorysessions.ControlRequest{})},
		{name: "InvocationMetric", typ: reflect.TypeOf(factorysessions.InvocationMetric{})},
		{name: "SessionIDGenerator", typ: reflect.TypeOf(factorysessions.SessionIDGenerator(nil))},
		{
			name: "ExecutionOpeningFileSystem",
			typ:  reflect.TypeOf((*factorysessions.ExecutionOpeningFileSystem)(nil)).Elem(),
		},
		{
			name: "DirectoryInspection",
			typ:  reflect.TypeOf((*factorysessions.DirectoryInspection)(nil)).Elem(),
		},
		{
			name: "RuntimePersistenceFileSystem",
			typ:  reflect.TypeOf((*factorysessions.RuntimePersistenceFileSystem)(nil)).Elem(),
		},
		{
			name: "ExecutionService",
			typ:  reflect.TypeOf((*factorysessions.ExecutionService)(nil)).Elem(),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.typ.PkgPath(); got != rootPkg {
				t.Fatalf("%s PkgPath = %q, want root-owned %q (nested re-export still present)", tc.name, got, rootPkg)
			}
		})
	}
}

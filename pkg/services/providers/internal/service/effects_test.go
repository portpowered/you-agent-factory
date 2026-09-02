package service

import (
	"reflect"
	"testing"
)

func TestReflectedRequestProjectsFactorySessionAsExecutionScope(t *testing.T) {
	t.Parallel()

	type targetRequest struct {
		FactorySessionID string
		ExecutionScopeID string
	}

	got, err := reflectedRequest(reflect.TypeFor[targetRequest](), CommandRequest{
		FactorySessionID: "factory-session-1",
	})
	if err != nil {
		t.Fatalf("reflectedRequest() error = %v", err)
	}
	request := got.Interface().(targetRequest)
	if request.FactorySessionID != "factory-session-1" || request.ExecutionScopeID != "factory-session-1" {
		t.Fatalf("reflectedRequest() = %#v, want session identity on both supported boundaries", request)
	}
}

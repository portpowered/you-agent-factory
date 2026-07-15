package token

import (
	"reflect"
	"testing"
)

func TestPreviousChainingTraceIDsIgnoresResourcesAndCanonicalizes(t *testing.T) {
	tokens := []Token{
		{Color: Color{DataType: DataTypeWork, CurrentChainingTraceID: "trace-b"}},
		{Color: Color{DataType: DataTypeResource, TraceID: "ignored"}},
		{Color: Color{DataType: DataTypeWork, TraceID: "trace-a"}},
		{Color: Color{DataType: DataTypeWork, TraceID: "trace-b"}},
	}
	if got, want := PreviousChainingTraceIDs(tokens), []string{"trace-a", "trace-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("PreviousChainingTraceIDs() = %#v, want %#v", got, want)
	}
}

func TestCurrentChainingTraceIDPrefersCustomerWork(t *testing.T) {
	tokens := []Token{
		{Color: Color{DataType: DataTypeWork, WorkTypeID: "system", TraceID: "trace-system"}},
		{Color: Color{DataType: DataTypeWork, WorkTypeID: "task", CurrentChainingTraceID: "trace-customer"}},
	}
	if got := CurrentChainingTraceID(tokens, "system"); got != "trace-customer" {
		t.Fatalf("CurrentChainingTraceID() = %q, want trace-customer", got)
	}
}

func TestChainingTraceDepthFromColorsUsesDeepestWorkInput(t *testing.T) {
	colors := []Color{
		{DataType: DataTypeResource, ChainingTraceDepth: 99},
		{DataType: DataTypeWork, ChainingTraceDepth: 2},
		{DataType: DataTypeWork, ChainingTraceDepth: 4},
	}
	if got := ChainingTraceDepthFromColors(colors); got != 5 {
		t.Fatalf("ChainingTraceDepthFromColors() = %d, want 5", got)
	}
}

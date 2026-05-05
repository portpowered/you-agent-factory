package interfaces

import "testing"

func TestCanonicalChainingTraceIDs_SortsAndDedupesNonEmptyValues(t *testing.T) {
	got := CanonicalChainingTraceIDs([]string{"trace-b", "", "trace-a", "trace-b", "trace-c"})
	want := []string{"trace-a", "trace-b", "trace-c"}
	assertCanonicalTraceIDs(t, got, want)
}

func TestPreviousChainingTraceIDsFromTokens_SingleInputFanOutPreservesOnePredecessor(t *testing.T) {
	got := PreviousChainingTraceIDsFromTokens([]Token{
		{Color: TokenColor{DataType: DataTypeWork, TraceID: "trace-parent"}},
		{Color: TokenColor{DataType: DataTypeResource, TraceID: "trace-resource-ignored"}},
	})
	want := []string{"trace-parent"}
	assertCanonicalTraceIDs(t, got, want)
}

func TestPreviousChainingTraceIDsFromTokens_MultiInputFanInReturnsSortedUniquePredecessors(t *testing.T) {
	got := PreviousChainingTraceIDsFromTokens([]Token{
		{Color: TokenColor{DataType: DataTypeWork, TraceID: "trace-z"}},
		{Color: TokenColor{DataType: DataTypeWork, TraceID: "trace-a"}},
		{Color: TokenColor{DataType: DataTypeWork, TraceID: "trace-z"}},
		{Color: TokenColor{DataType: DataTypeResource, TraceID: "trace-resource-ignored"}},
	})
	want := []string{"trace-a", "trace-z"}
	assertCanonicalTraceIDs(t, got, want)
}

func TestPreviousChainingTraceIDsFromTokens_PrefersCanonicalCurrentTraceOverLegacyTrace(t *testing.T) {
	got := PreviousChainingTraceIDsFromTokens([]Token{
		{Color: TokenColor{DataType: DataTypeWork, CurrentChainingTraceID: "chain-z", TraceID: "trace-a"}},
		{Color: TokenColor{DataType: DataTypeWork, CurrentChainingTraceID: "chain-a", TraceID: "trace-z"}},
	})
	want := []string{"chain-a", "chain-z"}
	assertCanonicalTraceIDs(t, got, want)
}

func TestPreviousChainingTraceIDsFromWorkItems_MultiInputFanInReturnsSortedUniquePredecessors(t *testing.T) {
	got := PreviousChainingTraceIDsFromWorkItems([]FactoryWorkItem{
		{ID: "work-2", TraceID: "trace-b"},
		{ID: "work-1", TraceID: "trace-a"},
		{ID: "work-3", TraceID: "trace-b"},
		{ID: "work-4"},
	})
	want := []string{"trace-a", "trace-b"}
	assertCanonicalTraceIDs(t, got, want)
}

func TestPreviousChainingTraceIDsFromTokenColors_MultiInputFanInReturnsSortedUniquePredecessors(t *testing.T) {
	got := PreviousChainingTraceIDsFromTokenColors([]TokenColor{
		{DataType: DataTypeWork, TraceID: "trace-z"},
		{DataType: DataTypeWork, TraceID: "trace-a"},
		{DataType: DataTypeWork, TraceID: "trace-z"},
		{DataType: DataTypeResource, TraceID: "trace-resource-ignored"},
	})
	want := []string{"trace-a", "trace-z"}
	assertCanonicalTraceIDs(t, got, want)
}

func TestCurrentChainingTraceIDFromTokens_PrefersCanonicalCurrentTraceOverLegacyTrace(t *testing.T) {
	got := CurrentChainingTraceIDFromTokens([]Token{
		{Color: TokenColor{DataType: DataTypeWork, WorkTypeID: "task", CurrentChainingTraceID: "chain-customer", TraceID: "trace-customer"}},
	})
	if got != "chain-customer" {
		t.Fatalf("current chaining trace ID = %q, want chain-customer", got)
	}
}

func TestCurrentChainingTraceIDFromTokens_PrefersCustomerWorkOverSystemTime(t *testing.T) {
	got := CurrentChainingTraceIDFromTokens([]Token{
		{Color: TokenColor{DataType: DataTypeWork, WorkTypeID: SystemTimeWorkTypeID, TraceID: "trace-system"}},
		{Color: TokenColor{DataType: DataTypeWork, WorkTypeID: "task", TraceID: "trace-customer"}},
	})
	if got != "trace-customer" {
		t.Fatalf("current chaining trace ID = %q, want trace-customer", got)
	}
}

func TestCurrentChainingTraceIDFromWorkItems_PrefersCustomerWorkOverSystemTime(t *testing.T) {
	got := CurrentChainingTraceIDFromWorkItems([]FactoryWorkItem{
		{WorkTypeID: SystemTimeWorkTypeID, CurrentChainingTraceID: "chain-system", TraceID: "trace-system"},
		{WorkTypeID: "task", CurrentChainingTraceID: "chain-customer", TraceID: "trace-customer"},
	})
	if got != "chain-customer" {
		t.Fatalf("current chaining trace ID from work items = %q, want chain-customer", got)
	}
}

func TestChainingTraceDepthFromTokenColors_UsesDeepestNonResourceAncestor(t *testing.T) {
	got := ChainingTraceDepthFromTokenColors([]TokenColor{
		{DataType: DataTypeResource, WorkTypeID: "slot", ChainingTraceDepth: 99},
		{DataType: DataTypeWork, WorkTypeID: "task", ChainingTraceDepth: 2, TraceID: "trace-a"},
		{DataType: DataTypeWork, WorkTypeID: "task", ChainingTraceDepth: 4, TraceID: "trace-b"},
	})
	if got != 5 {
		t.Fatalf("chaining trace depth = %d, want 5", got)
	}
}

func TestChainingTraceDepthForTokenColor_AndWorkItem_FallbackToInitialDepth(t *testing.T) {
	tokenDepth := ChainingTraceDepthForTokenColor(TokenColor{CurrentChainingTraceID: "chain-1"})
	if tokenDepth != 1 {
		t.Fatalf("token chaining trace depth = %d, want 1", tokenDepth)
	}

	workDepth := ChainingTraceDepthForWorkItem(FactoryWorkItem{TraceID: "trace-1"})
	if workDepth != 1 {
		t.Fatalf("work item chaining trace depth = %d, want 1", workDepth)
	}
}

func assertCanonicalTraceIDs(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("trace ID count = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("trace IDs = %#v, want %#v", got, want)
		}
	}
}

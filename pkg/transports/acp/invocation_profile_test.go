package acp

import (
	"context"
	"testing"
)

func TestInvocationProfileRoundTripsThroughContext(t *testing.T) {
	t.Parallel()

	want := InvocationProfile{HomeDir: "isolated-home", WorkerModelProvider: "codex", WorkerModel: "gpt-5"}
	got, ok := InvocationProfileFromContext(WithInvocationProfile(context.Background(), want))
	if !ok || got != want {
		t.Fatalf("InvocationProfileFromContext() = (%+v, %t), want (%+v, true)", got, ok, want)
	}
}

func TestInvocationProfileFromNilContextIsAbsent(t *testing.T) {
	t.Parallel()

	got, ok := InvocationProfileFromContext(nil)
	if ok || got != (InvocationProfile{}) {
		t.Fatalf("InvocationProfileFromContext(nil) = (%+v, %t), want zero profile and false", got, ok)
	}
}

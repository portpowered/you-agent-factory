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

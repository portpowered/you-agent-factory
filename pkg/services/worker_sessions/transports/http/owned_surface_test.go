package http

import (
	"slices"
	"testing"
)

func TestOwnedHTTPSurfaceIncludesWorkerSessionList(t *testing.T) {
	want := []string{"listWorkerSessionsBySessionId"}
	if !slices.Equal(OwnedHTTPOperationIDs, want) {
		t.Fatalf("OwnedHTTPOperationIDs = %#v, want %#v", OwnedHTTPOperationIDs, want)
	}
}

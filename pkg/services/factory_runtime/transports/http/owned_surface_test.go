package http

import (
	"slices"
	"testing"
)

func TestOwnedHTTPSurfaceIncludesStatusReads(t *testing.T) {
	t.Parallel()

	want := []string{"getStatus", "getStatusBySessionId", "moveWorkBySessionId"}
	if !slices.Equal(OwnedHTTPOperationIDs, want) {
		t.Fatalf("OwnedHTTPOperationIDs = %#v, want %#v", OwnedHTTPOperationIDs, want)
	}
}

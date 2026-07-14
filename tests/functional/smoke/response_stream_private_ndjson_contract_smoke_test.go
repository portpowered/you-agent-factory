package smoke

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream/removalgate"
)

// TestResponseStreamPrivateNDJSONContractSmoke is the maintained functional
// entrypoint for Batch 09 Story 003 private-record contract negatives.
func TestResponseStreamPrivateNDJSONContractSmoke(t *testing.T) {
	if err := removalgate.AssertPrivateNDJSONRecordTypesRejected(); err != nil {
		t.Fatal(err)
	}
}

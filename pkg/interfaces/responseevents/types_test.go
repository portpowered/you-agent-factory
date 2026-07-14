package responseevents_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces/responseevents"
)

func TestCloneDraftCopiesPayload(t *testing.T) {
	source := responseevents.Draft{Payload: []byte(`{"message":"safe"}`)}
	cloned := responseevents.CloneDraft(source)

	cloned.Payload[0] = '['
	if source.Payload[0] != '{' {
		t.Fatalf("source payload mutated through clone: %q", source.Payload)
	}
}

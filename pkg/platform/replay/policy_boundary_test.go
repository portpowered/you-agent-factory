package replay

import (
	"os"
	"strings"
	"testing"
)

func TestPlatformReplayStorageContainsNoDomainReplayPolicy(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("storage.go")
	if err != nil {
		t.Fatalf("read storage.go: %v", err)
	}
	text := string(source)
	for _, forbidden := range []string{
		"pkg/services/",
		"FactoryEvent",
		"NormalizeHistoricalFailureDetails",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("Platform replay storage contains domain policy marker %q; interpretation belongs to Recordings", forbidden)
		}
	}
}

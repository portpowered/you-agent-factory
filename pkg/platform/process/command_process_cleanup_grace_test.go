package process

import (
	"testing"
	"time"
)

func TestDefaultPostRunCleanupGracePeriod(t *testing.T) {
	if defaultPostRunCleanupGracePeriod != 10*time.Second {
		t.Fatalf("defaultPostRunCleanupGracePeriod = %v, want 10s", defaultPostRunCleanupGracePeriod)
	}
}

func TestPostRunCleanupGracePeriod_TestHookOverridesDefault(t *testing.T) {
	t.Cleanup(func() {
		postRunCleanupGracePeriodForTest = 0
	})

	if postRunCleanupGracePeriod() != defaultPostRunCleanupGracePeriod {
		t.Fatalf("postRunCleanupGracePeriod() = %v, want default %v", postRunCleanupGracePeriod(), defaultPostRunCleanupGracePeriod)
	}

	postRunCleanupGracePeriodForTest = 25 * time.Millisecond
	if postRunCleanupGracePeriod() != 25*time.Millisecond {
		t.Fatalf("postRunCleanupGracePeriod() = %v, want test override 25ms", postRunCleanupGracePeriod())
	}
}

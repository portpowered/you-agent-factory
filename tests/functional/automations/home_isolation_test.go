package automations

import (
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/testhome"
)

// TestMain keeps this package's tests off the real user profile.
func TestMain(m *testing.M) {
	cleanup := testhome.IsolateHomeMain()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

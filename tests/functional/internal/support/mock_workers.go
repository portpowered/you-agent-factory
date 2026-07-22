package support

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// WriteMockWorkersConfig materializes the customer-facing --with-mock-workers
// input used by a root-built process.
func WriteMockWorkersConfig(t testing.TB, config *workers.MockWorkersConfig) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mock-workers.json")
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal mock workers config: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write mock workers config: %v", err)
	}
	return path
}

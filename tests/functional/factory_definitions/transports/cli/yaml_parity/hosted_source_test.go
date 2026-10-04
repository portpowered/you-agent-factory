package yaml_parity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Only host startup owns Current Factory. The idle host has no seeded Work;
// independent source journeys run in parallel in their explicit sessions.
func startAuthoredSourceHost(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "authored-source-idle-host",
		"workTypes": []map[string]any{{
			"name": "idle",
			"states": []map[string]string{
				{"name": "ready", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
			},
		}},
	})
	env := customerEnvironment(t.TempDir())
	support.InitializeCustomerHomeWithProcess(t, yamlParityCLIProcess, env, dir)
	ctx, cancel := context.WithCancel(t.Context())
	inputs := support.FakeInputs(ctx, []string{
		"you", "run", "--dir", dir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = dir
	done := make(chan error, 1)
	go func() { done <- yamlParityCLIProcess.Execute(inputs.Input) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("stop authored-source host: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("authored-source host did not stop")
		}
	})
	baseURL, err := yamlParityAPI.WaitForBaseURL(15 * time.Second)
	if err != nil {
		t.Fatalf("start authored-source host: %v", err)
	}
	return baseURL
}

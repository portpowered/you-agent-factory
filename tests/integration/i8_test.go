package integration

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/integration/harness"
)

const i8Timeout = 90 * time.Second

//go:embed testdata/i8/valid/*.json testdata/i8/invalid/*.json
var i8Corpus embed.FS

// TestI8FactoryConfigValidation proves the installed binary's validate-only
// verdict across a compact Factory corpus. The corpus is copied out of the
// checkout before any child process starts so the harness audits only the
// installed-binary boundary.
func TestI8FactoryConfigValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), i8Timeout)
	defer cancel()

	run, err := harness.New(repositoryRoot(t))
	if err != nil {
		t.Fatalf("create installed-binary harness: %v", err)
	}
	defer func() {
		if err := run.Close(); err != nil {
			t.Errorf("close installed-binary harness: %v", err)
		}
	}()

	cases := []struct {
		name  string
		valid bool
	}{
		{name: "valid/minimal.json", valid: true},
		{name: "valid/logical-move.json", valid: true},
		{name: "valid/resource-backed.json", valid: true},
		{name: "invalid/malformed.json", valid: false},
		{name: "invalid/unknown-output-state.json", valid: false},
		{name: "invalid/duplicate-worker.json", valid: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateFixture(ctx, run, tc.name)
			if err != nil {
				t.Fatalf("validate %s: %v", tc.name, err)
			}
			if got != tc.valid {
				t.Fatalf("validation verdict = %t, want %t", got, tc.valid)
			}
		})
	}
}

func validateFixture(ctx context.Context, run *harness.Harness, name string) (bool, error) {
	invocation, err := run.NewInvocation()
	if err != nil {
		return false, err
	}
	defer invocation.Close()

	fixture, err := i8Corpus.ReadFile(filepath.ToSlash(filepath.Join("testdata/i8", name)))
	if err != nil {
		return false, fmt.Errorf("read embedded fixture: %w", err)
	}
	env := invocation.Environment()
	factoryPath := filepath.Join(env.WorkingDirectory, "factory"+filepath.Ext(name))
	if err := os.WriteFile(factoryPath, fixture, 0o600); err != nil {
		return false, fmt.Errorf("stage fixture: %w", err)
	}

	result, err := invocation.Run(ctx, "factory", "config", "validate", factoryPath)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", ".."))
}

package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	policy "github.com/portpowered/infinite-you/pkg/services/work/internal/invocationreturnpolicy"
	invocationwire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/invocation_preparation/wire"
)

func TestNewInvocationInputPreparationReadsInjectedRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "long prompt.txt")
	want := "  line one\r\nline two — 東京\r\n"
	if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	prepared, err := invocationwire.NewInvocationInputAdapter(policy.NewInvocationInputPreparation(os.ReadFile, os.Stat)).PrepareInvocationInput(
		context.Background(),
		work.InvocationInputPreparationRequest{FilePath: &path},
	)
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.Source != work.InputSourceFileText || prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != want {
		t.Fatalf("prepared = %#v, want exact file-backed text", prepared)
	}
}

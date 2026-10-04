package wire

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/invocationreturnpolicy"
)

func TestInvocationPolicyUsesSelectedFileEdges(t *testing.T) {
	t.Parallel()
	path := "input.txt"
	policy := NewInvocationInputPolicy(func(got string) ([]byte, error) {
		if got != path {
			t.Fatalf("file path=%q", got)
		}
		return []byte("selected input"), nil
	}, func(got string) (fs.FileInfo, error) {
		return fstest.MapFS{path: &fstest.MapFile{Data: []byte("selected input")}}.Stat(got)
	})
	got, err := policy.PrepareInvocationInput(t.Context(), invocationreturnpolicy.InvocationInputPreparationRequest{FilePath: &path})
	if err != nil || got.ResolvedInput == nil || got.ResolvedInput.Text != "selected input" {
		t.Fatalf("prepared=%#v,error=%v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := policy.PrepareInvocationInput(ctx, invocationreturnpolicy.InvocationInputPreparationRequest{FilePath: &path}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled preparation=%v", err)
	}
}

package reclamation

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestCacheReclamationRemovalPreservesPeerAndReportsOnlyVerifiedBytes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	selected := filepath.Join(root, "selected")
	peer := filepath.Join(root, "peer")
	for _, path := range []string{selected, peer} {
		if err := os.MkdirAll(filepath.Join(path, "nested"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "nested", "model"), []byte("model"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	policy := removalPolicy(os.Remove)
	bytes, err := policy.MeasureRevisionBytes(t.Context(), selected)
	if err != nil || bytes != 5 {
		t.Fatalf("measure = %d, %v", bytes, err)
	}
	got, err := policy.Apply(t.Context(), Plan{candidates: []candidate{{path: selected, bytes: bytes}, {path: peer, bytes: bytes, shared: true}}})
	if err != nil || got != 5 {
		t.Fatalf("apply = %d, %v", got, err)
	}
	if _, err := os.Stat(selected); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("selected stat = %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(peer, "nested", "model")); err != nil || string(body) != "model" {
		t.Fatalf("peer = %q, %v", body, err)
	}
}

func TestCacheReclamationCancellationAndPartialFailureReturnNoSuccessBytes(t *testing.T) {
	t.Parallel()
	for _, cancelDuringRemoval := range []bool{false, true} {
		t.Run(map[bool]string{false: "file failure", true: "cancellation"}[cancelDuringRemoval], func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := filepath.Join(root, "model")
			if err := os.WriteFile(path, []byte("model"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			cause := errors.New("owned cancellation")
			policy := removalPolicy(func(path string) error {
				if cancelDuringRemoval {
					cancel(cause)
					return os.Remove(path)
				}
				return errors.New("denied")
			})
			bytes, err := policy.Apply(ctx, Plan{candidates: []candidate{{path: root, bytes: 5}}})
			want := models.ErrModelCacheRemovalFailed
			if cancelDuringRemoval {
				want = context.Canceled
			}
			if bytes != 0 || !errors.Is(err, want) {
				t.Fatalf("apply = %d, %v; want zero and %v", bytes, err, want)
			}
			if cancelDuringRemoval && !errors.Is(err, cause) {
				t.Fatalf("lost cancellation cause: %v", err)
			}
			if _, err := os.Stat(root); err != nil {
				t.Fatalf("root removed after failure: %v", err)
			}
		})
	}
}

func TestCacheReclamationUnknownSourceCannotAuthorizeRemoval(t *testing.T) {
	t.Parallel()
	files := []models.AssetRequirement{{Name: "model", Bytes: 5}}
	identity, _ := artifactIdentity(modelKind, "selected", files)
	items := []candidate{{kind: modelKind, identity: identity, artifacts: files}}
	err := markCandidatesShared(items, ManagedMetadata{Files: []ManagedFile{{Path: "model", Bytes: 5}}}, "unknown-alias", unknownSources{})
	if err == nil || items[0].shared {
		t.Fatalf("unknown source census = %v, %#v", err, items)
	}
	if !errors.Is(uncertain("selected", err), models.ErrModelCacheReferenceUncertain) {
		t.Fatal("uncertainty lost its typed classification")
	}
}

func TestCacheReclamationRemovalFailuresPreserveClassificationAndCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("selected filesystem failure")
	for _, operation := range []string{"read revision", "inspect entry", "remove path", "verify removal"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			policy := New(func(string) ([]os.DirEntry, error) { return nil, cause }, nil, nil, nil,
				func(string) error { return cause }, nil, unknownSources{}, reclamationContextError)
			var err error
			switch operation {
			case "read revision":
				err = policy.RemoveManagedTree(t.Context(), "selected")
			case "inspect entry":
				err = policy.removeManagedEntry(t.Context(), "selected", failedRemovalEntry{cause: cause})
			case "remove path":
				err = policy.removeManagedPath("selected")
			case "verify removal":
				err = policy.VerifyManagedPathRemoved(t.Context(), "selected", "revision")
			}
			if !errors.Is(err, models.ErrModelCacheRemovalFailed) || !errors.Is(err, cause) {
				t.Fatalf("%s error = %v, want removal classification and filesystem cause", operation, err)
			}
		})
	}
}

type failedRemovalEntry struct{ cause error }

func (failedRemovalEntry) Name() string                     { return "model" }
func (failedRemovalEntry) IsDir() bool                      { return false }
func (failedRemovalEntry) Type() os.FileMode                { return 0 }
func (entry failedRemovalEntry) Info() (os.FileInfo, error) { return nil, entry.cause }

func TestCacheReclamationReleasesEveryAcquiredLockAfterFailure(t *testing.T) {
	t.Parallel()
	policy := removalPolicy(os.Remove)
	var order []int
	primary := errors.New("primary")
	err := policy.CloseLocks([]io.Closer{closeFunc(func() error { order = append(order, 1); return nil }), closeFunc(func() error { order = append(order, 2); return errors.New("close failed") })}, primary)
	if !errors.Is(err, primary) || !errors.Is(err, models.ErrModelCacheRemovalFailed) || len(order) != 2 || order[0] != 2 || order[1] != 1 {
		t.Fatalf("close = %v, order = %v", err, order)
	}
}

func removalPolicy(remove func(string) error) *Service {
	return New(os.ReadDir, os.ReadFile, func(path string) (io.ReadCloser, error) { return os.Open(path) }, os.Stat, remove, nil, unknownSources{}, reclamationContextError)
}

func reclamationContextError(ctx context.Context) error {
	return errors.Join(ctx.Err(), context.Cause(ctx))
}

type unknownSources struct{}

func (unknownSources) ManagedSourceIdentity(string) (string, bool) { return "", false }

type closeFunc func() error

func (close closeFunc) Close() error { return close() }

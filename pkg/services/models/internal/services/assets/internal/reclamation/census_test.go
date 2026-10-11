package reclamation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestCacheReclamationCensusFailsClosedOnUnprovedOwnership(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"corrupt reference", "unknown source", "unverified snapshot", "unsafe backend"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			root, selected, snapshot, metadata := writeCensusFixture(t)
			peer := filepath.Join(root, "peer")
			writeCensusJSON(t, filepath.Join(peer, ".managed-cache.json"), metadata)
			switch failure {
			case "corrupt reference":
				if err := os.WriteFile(filepath.Join(peer, ".managed-cache.json"), []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unknown source":
				metadata.ModelName = "unknown-alias"
				writeCensusJSON(t, filepath.Join(peer, ".managed-cache.json"), metadata)
			case "unverified snapshot":
				if err := os.WriteFile(filepath.Join(snapshot, "model"), []byte("wrong"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unsafe backend":
				metadata.Backend = &BackendMetadata{CachePath: "../outside"}
			}
			coordination := &censusCoordination{}
			policy := censusPolicy(coordination)
			plan, locks, err := policy.PlanAndLockCandidates(t.Context(), root, selected, "selected", metadata, "source")
			if !errors.Is(err, models.ErrModelCacheReferenceUncertain) || len(plan.candidates) != 0 {
				t.Fatalf("unproved census = %#v, %v; want typed uncertainty without removal plan", plan, err)
			}
			if closeErr := policy.CloseLocks(locks, err); !errors.Is(closeErr, models.ErrModelCacheReferenceUncertain) {
				t.Fatal(closeErr)
			}
			for _, path := range []string{selected, peer, snapshot} {
				if _, statErr := os.Stat(path); statErr != nil {
					t.Fatalf("uncertain census changed %s: %v", path, statErr)
				}
			}
		})
	}
}

func TestCacheReclamationReplansAfterSourceLockAndRetainsPublishedPeer(t *testing.T) {
	t.Parallel()
	root, selected, snapshot, metadata := writeCensusFixture(t)
	coordination := &censusCoordination{onLock: func(ctx context.Context, path string) error {
		writeCensusJSON(t, filepath.Join(root, "peer", ".managed-cache.json"), metadata)
		return nil
	}}
	policy := censusPolicy(coordination)
	plan, locks, err := policy.PlanAndLockCandidates(t.Context(), root, selected, "selected", metadata, "source")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := policy.CloseLocks(locks, nil); err != nil {
			t.Error(err)
		}
		if coordination.acquired != coordination.closed {
			t.Errorf("locks acquired=%d closed=%d", coordination.acquired, coordination.closed)
		}
	}()
	bytes, err := policy.MeasureRevisionBytes(t.Context(), snapshot)
	if err != nil || bytes == 0 || plan.Retained != bytes {
		t.Fatalf("replanned shared bytes=%d retained=%d err=%v", bytes, plan.Retained, err)
	}
	if got, err := policy.Apply(t.Context(), plan); err != nil || got != 0 {
		t.Fatalf("shared apply=%d, %v", got, err)
	}
	if body, err := os.ReadFile(filepath.Join(snapshot, "model")); err != nil || string(body) != "model" {
		t.Fatalf("published peer lost cache: %q, %v", body, err)
	}
}

func TestCacheReclamationCanceledLockReleasesReferenceAndPreservesCause(t *testing.T) {
	t.Parallel()
	root, selected, snapshot, metadata := writeCensusFixture(t)
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	cause := errors.New("selected lock canceled")
	coordination := &censusCoordination{onLock: func(ctx context.Context, path string) error {
		if filepath.Base(path) == referenceLock {
			return nil
		}
		cancel(cause)
		return reclamationContextError(ctx)
	}}
	policy := censusPolicy(coordination)
	reference, err := policy.LockReferences(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	_, locks, err := policy.PlanAndLockCandidates(ctx, root, selected, "selected", metadata, "source")
	err = policy.CloseLocks(append([]io.Closer{reference}, locks...), err)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || coordination.acquired != 1 || coordination.closed != 1 {
		t.Fatalf("canceled ownership err=%v acquired=%d closed=%d", err, coordination.acquired, coordination.closed)
	}
	if body, err := os.ReadFile(filepath.Join(snapshot, "model")); err != nil || string(body) != "model" {
		t.Fatalf("cancel changed snapshot: %q, %v", body, err)
	}
}

func TestCacheReclamationPreCancellationDoesNotInspectOrDelete(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	policy := New(func(string) ([]os.DirEntry, error) { t.Fatal("canceled operation read directory"); return nil, nil }, nil, nil, nil,
		func(string) error { t.Fatal("canceled operation deleted path"); return nil }, nil, nil, reclamationContextError)
	if err := policy.RemoveManagedTree(ctx, "selected"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if bytes, err := policy.MeasureRevisionBytes(ctx, "selected"); bytes != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("measure=%d, %v", bytes, err)
	}
}

// No OS symlink privilege is required: the injected directory entry reports
// a link, and neither Info nor traversal of its target is permitted.
func TestCacheReclamationRemovesLinkWithoutFollowingTarget(t *testing.T) {
	t.Parallel()
	var removed []string
	policy := New(func(path string) ([]os.DirEntry, error) {
		if path != "selected" {
			t.Fatalf("followed link target: %s", path)
		}
		return []os.DirEntry{censusLinkEntry{}}, nil
	}, nil, nil, nil, func(path string) error { removed = append(removed, path); return nil }, nil, nil, reclamationContextError)
	if bytes, err := policy.MeasureRevisionBytes(t.Context(), "selected"); bytes != 0 || err != nil {
		t.Fatalf("link bytes=%d, %v", bytes, err)
	}
	if err := policy.RemoveManagedTree(t.Context(), "selected"); err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 || removed[0] != filepath.Join("selected", "link") || removed[1] != "selected" {
		t.Fatalf("removed=%v", removed)
	}
}

type censusLinkEntry struct{}

func (censusLinkEntry) Name() string               { return "link" }
func (censusLinkEntry) IsDir() bool                { return false }
func (censusLinkEntry) Type() os.FileMode          { return os.ModeSymlink }
func (censusLinkEntry) Info() (os.FileInfo, error) { panic("symlink target inspected") }

type censusSources struct{}

func (censusSources) ManagedSourceIdentity(name string) (string, bool) {
	return "source", name != "unknown-alias"
}

type censusCoordination struct {
	onLock           func(context.Context, string) error
	acquired, closed int
}

func (c *censusCoordination) Lock(ctx context.Context, path string) (io.Closer, error) {
	if c.onLock != nil {
		if err := c.onLock(ctx, path); err != nil {
			return nil, err
		}
	}
	c.acquired++
	return closeFunc(func() error { c.closed++; return nil }), nil
}
func censusPolicy(coordination *censusCoordination) *Service {
	return New(os.ReadDir, os.ReadFile, func(path string) (io.ReadCloser, error) { return os.Open(path) }, os.Stat, os.Remove, coordination, censusSources{}, reclamationContextError)
}
func writeCensusFixture(t *testing.T) (string, string, string, ManagedMetadata) {
	t.Helper()
	root := t.TempDir()
	selected := filepath.Join(root, "selected")
	body := []byte("model")
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	artifacts := []models.AssetRequirement{{Name: "model", Bytes: int64(len(body)), SHA256: digest}}
	identity, key := artifactIdentity(modelKind, "source", artifacts)
	snapshot := filepath.Join(root, contentDirectory, modelKind, identity)
	writeCensusJSON(t, filepath.Join(snapshot, metadataName), SnapshotMetadata{Kind: modelKind, Identity: key, Source: "source", SourceKey: "source", Artifacts: artifacts})
	if err := os.WriteFile(filepath.Join(snapshot, "model"), body, 0600); err != nil {
		t.Fatal(err)
	}
	metadata := ManagedMetadata{ModelName: "selected", Revision: "revision", Files: []ManagedFile{{Path: "model", Bytes: int64(len(body)), SHA256: digest}}}
	writeCensusJSON(t, filepath.Join(selected, ".managed-cache.json"), metadata)
	return root, selected, snapshot, metadata
}
func writeCensusJSON(t *testing.T, path string, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
}

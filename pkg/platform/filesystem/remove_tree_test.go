package filesystem

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalRemoveTreeRemovesOnlyNamedTargetAndIsIdempotent(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	target := filepath.Join(parent, "model")
	if err := os.MkdirAll(filepath.Join(target, "revision", "nested"), 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "revision", "nested", "weights.bin"), []byte("asset"), 0o644); err != nil {
		t.Fatalf("write target asset: %v", err)
	}
	sibling := filepath.Join(parent, "sibling")
	if err := os.WriteFile(sibling, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write sibling: %v", err)
	}

	changed, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err != nil || !changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want changed", changed, err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target = %v, want absent", err)
	}
	if _, err := os.Stat(parent); err != nil {
		t.Fatalf("parent = %v, want preserved", err)
	}
	if body, err := os.ReadFile(sibling); err != nil || string(body) != "preserve" {
		t.Fatalf("sibling = %q, %v; want preserved", body, err)
	}

	changed, err = (Local{}).RemoveTree(context.Background(), parent, "model")
	if err != nil || changed {
		t.Fatalf("repeated RemoveTree = changed %t, err %v; want unchanged", changed, err)
	}
}

func TestLocalRemoveTreeFailsClosedOnTargetDirectoryLink(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	outside := t.TempDir()
	marker := filepath.Join(outside, "outside-marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write outside marker: %v", err)
	}
	link := filepath.Join(parent, "model")
	createDirectoryLink(t, link, outside)

	changed, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err == nil || changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want fail-closed unchanged", changed, err)
	}
	assertFileBody(t, marker, "preserve")
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("target link = %v, want untouched", err)
	}
}

func TestLocalRemoveTreeFailsClosedOnCacheParentLink(t *testing.T) {
	t.Parallel()

	linkParent := t.TempDir()
	outside := t.TempDir()
	model := filepath.Join(outside, "model")
	if err := os.MkdirAll(model, 0o755); err != nil {
		t.Fatalf("create outside model: %v", err)
	}
	marker := filepath.Join(model, "outside-marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write outside marker: %v", err)
	}
	parentLink := filepath.Join(linkParent, "cache")
	createDirectoryLink(t, parentLink, outside)

	changed, err := (Local{}).RemoveTree(context.Background(), parentLink, "model")
	if err == nil || changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want fail-closed unchanged", changed, err)
	}
	assertFileBody(t, marker, "preserve")
	if _, err := os.Lstat(parentLink); err != nil {
		t.Fatalf("cache parent link = %v, want untouched", err)
	}
}

func TestLocalRemoveTreeUnlinksChildLinkWithoutFollowingOutside(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	target := filepath.Join(parent, "model")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	outside := t.TempDir()
	marker := filepath.Join(outside, "outside-marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write outside marker: %v", err)
	}
	createDirectoryLink(t, filepath.Join(target, "redirect"), outside)

	changed, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err != nil || !changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want successful removal", changed, err)
	}
	assertFileBody(t, marker, "preserve")
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target = %v, want absent", err)
	}
}

func TestLocalRemoveTreeDoesNotFollowReplacedDirectoryLink(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	target := filepath.Join(parent, "model")
	child := filepath.Join(target, "revision")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("create child: %v", err)
	}
	outside := t.TempDir()
	marker := filepath.Join(outside, "outside-marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write outside marker: %v", err)
	}
	original := filepath.Join(target, "revision-original")
	if err := os.Rename(child, original); err != nil {
		t.Fatalf("replace child before removal: %v", err)
	}
	createDirectoryLink(t, child, outside)

	changed, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err != nil || !changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want successful removal", changed, err)
	}
	assertFileBody(t, marker, "preserve")
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target = %v, want absent", err)
	}
}

func TestRemoveTreeContentsHasDeterministicPartialFailureSemantics(t *testing.T) {
	t.Parallel()

	failure := errors.New("stop at b")
	directory := &recordingTreeDirectory{
		entries:   []treeEntry{{name: "b"}, {name: "a"}},
		removeErr: map[string]error{"b": failure},
	}
	changed, err := removeTreeContents(context.Background(), directory)
	if !changed || !errors.Is(err, failure) {
		t.Fatalf("removeTreeContents = changed %t, err %v; want partial failure", changed, err)
	}
	if got, want := directory.removed, []string{"a", "b"}; !equalStrings(got, want) {
		t.Fatalf("removal order = %#v, want %#v", got, want)
	}
}

func TestRemoveTreeContentsStopsAfterCancellationBetweenEffects(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	directory := &recordingTreeDirectory{
		entries: []treeEntry{{name: "b"}, {name: "a"}},
		onRemove: func(name string) {
			if name == "a" {
				cancel()
			}
		},
	}
	changed, err := removeTreeContents(ctx, directory)
	if !changed || !errors.Is(err, context.Canceled) {
		t.Fatalf("removeTreeContents = changed %t, err %v; want cancellation after first effect", changed, err)
	}
	if got, want := directory.removed, []string{"a"}; !equalStrings(got, want) {
		t.Fatalf("removal order = %#v, want %#v", got, want)
	}
}

func TestRemoveTreeContentsUnlinksEntryWhenDirectoryIsReplaced(t *testing.T) {
	t.Parallel()

	replacement := errors.New("directory was replaced before no-follow open")
	directory := &recordingTreeDirectory{
		entries: []treeEntry{{name: "revision", isDir: true}},
		openErr: map[string]error{"revision": replacement},
	}
	changed, err := removeTreeContents(context.Background(), directory)
	if err != nil || !changed {
		t.Fatalf("removeTreeContents = changed %t, err %v; want replacement entry unlinked", changed, err)
	}
	if got, want := directory.removed, []string{"revision"}; !equalStrings(got, want) {
		t.Fatalf("replacement removal order = %#v, want %#v", got, want)
	}
}

type recordingTreeDirectory struct {
	entries   []treeEntry
	removed   []string
	removeErr map[string]error
	openErr   map[string]error
	onRemove  func(string)
}

func (directory *recordingTreeDirectory) ReadDir() ([]treeEntry, error) {
	return append([]treeEntry(nil), directory.entries...), nil
}

func (directory *recordingTreeDirectory) OpenDirectory(name string) (treeDirectory, error) {
	if err := directory.openErr[name]; err != nil {
		return nil, err
	}
	return nil, errors.New("unexpected directory open")
}

func (directory *recordingTreeDirectory) Remove(name string) error {
	directory.removed = append(directory.removed, name)
	if directory.onRemove != nil {
		directory.onRemove(name)
	}
	if err := directory.removeErr[name]; err != nil {
		return err
	}
	return nil
}

func (*recordingTreeDirectory) Close() error { return nil }

func createDirectoryLink(t *testing.T, link, target string) {
	t.Helper()
	if err := os.Symlink(target, link); err == nil {
		t.Cleanup(func() { _ = os.Remove(link) })
		return
	} else if runtime.GOOS != "windows" {
		t.Skipf("directory symlink capability unavailable on %s: %v", runtime.GOOS, err)
	} else {
		symlinkErr := err
		junctionErr := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target).Run()
		if junctionErr == nil {
			t.Cleanup(func() { _ = os.Remove(link) })
			return
		}
		t.Skipf("directory symlink/reparse capability unavailable on %s: symlink=%v; junction=%v", runtime.GOOS, symlinkErr, junctionErr)
	}
}

func assertFileBody(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if string(body) != want {
		t.Fatalf("%q = %q, want %q", path, body, want)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

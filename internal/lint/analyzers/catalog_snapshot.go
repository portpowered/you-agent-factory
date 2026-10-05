package analyzers

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const catalogPackagePath = "packages/packaged-factories/"

type catalogFile struct {
	data []byte
	mode fs.FileMode
}

// catalogSnapshot is invocation-local. Projection can enumerate this memory
// filesystem, but never receives a live disk filesystem.
type catalogSnapshot map[string]catalogFile

func loadCatalogSnapshot(directory string) (catalogSnapshot, error) {
	cmd := exec.Command("git", "-C", directory, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("resolve catalog workspace: %w", err)
	}
	root := strings.TrimSpace(string(out))
	// No exclude-standard: ignored generated files also constitute drift.
	cmd = exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--",
		catalogPackagePath+"factories", catalogPackagePath+"schemas", catalogPackagePath+"generated")
	out, err = cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("enumerate catalog inputs: %w", err)
	}
	return readCatalogSnapshot(out, func(name string) (catalogFile, error) {
		// Reject symlink parents before reading; they could escape the workspace.
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			info, failure := os.Lstat(filepath.Join(root, filepath.FromSlash(parent)))
			if failure != nil {
				return catalogFile{}, failure
			}
			if !info.IsDir() {
				return catalogFile{}, fmt.Errorf("non-directory catalog parent %s", parent)
			}
		}
		filename := filepath.Join(root, filepath.FromSlash(name))
		info, failure := os.Lstat(filename)
		if failure != nil {
			return catalogFile{}, failure
		}
		file := catalogFile{mode: info.Mode()}
		if info.Mode().IsRegular() {
			file.data, failure = os.ReadFile(filename)
		}
		return file, failure
	})
}

func readCatalogSnapshot(inventory []byte, read func(string) (catalogFile, error)) (catalogSnapshot, error) {
	if len(inventory) > 0 && inventory[len(inventory)-1] != 0 {
		return nil, fmt.Errorf("catalog inventory is not NUL terminated")
	}
	result := catalogSnapshot{}
	for _, name := range strings.Split(string(inventory), "\x00") {
		if name == "" {
			continue
		}
		if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") || !strings.HasPrefix(name, catalogPackagePath) {
			return nil, fmt.Errorf("unsafe catalog inventory path %q", name)
		}
		relative := strings.TrimPrefix(name, catalogPackagePath)
		if !strings.HasPrefix(relative, "factories/") && !strings.HasPrefix(relative, "schemas/") && !strings.HasPrefix(relative, "generated/") {
			return nil, fmt.Errorf("outside catalog input inventory: %s", name)
		}
		file, err := read(name)
		if os.IsNotExist(err) {
			continue // Tracked deletions are absent from the workspace snapshot.
		}
		if err != nil {
			return nil, fmt.Errorf("read catalog input %s: %w", name, err)
		}
		file.data = bytes.Clone(file.data)
		result[relative] = file
	}
	return result, nil
}

func (s catalogSnapshot) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if file, ok := s[name]; ok {
		if !file.mode.IsRegular() {
			return nil, fmt.Errorf("catalog input %s is not a regular file", name)
		}
		return &catalogMemoryFile{Reader: bytes.NewReader(file.data), info: catalogInfo{name: path.Base(name), size: int64(len(file.data)), mode: file.mode}}, nil
	}
	entries, err := s.ReadDir(name)
	if err != nil {
		return nil, err
	}
	return &catalogMemoryFile{Reader: bytes.NewReader(nil), info: catalogInfo{name: path.Base(name), mode: fs.ModeDir | 0o755}, entries: entries}, nil
}

func (s catalogSnapshot) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	prefix := name + "/"
	if name == "." {
		prefix = ""
	}
	children := map[string]catalogInfo{}
	for filename, file := range s {
		if !strings.HasPrefix(filename, prefix) {
			continue
		}
		relative := strings.TrimPrefix(filename, prefix)
		child, _, nested := strings.Cut(relative, "/")
		info := catalogInfo{name: child, size: int64(len(file.data)), mode: file.mode}
		if nested {
			info.size, info.mode = 0, fs.ModeDir|0o755
		}
		children[child] = info
	}
	if len(children) == 0 && name != "." {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	names := make([]string, 0, len(children))
	for child := range children {
		names = append(names, child)
	}
	sort.Strings(names)
	entries := make([]fs.DirEntry, 0, len(names))
	for _, child := range names {
		entries = append(entries, children[child])
	}
	return entries, nil
}

type catalogInfo struct {
	name string
	size int64
	mode fs.FileMode
}

func (i catalogInfo) Name() string               { return i.name }
func (i catalogInfo) Size() int64                { return i.size }
func (i catalogInfo) Mode() fs.FileMode          { return i.mode }
func (i catalogInfo) ModTime() time.Time         { return time.Time{} }
func (i catalogInfo) IsDir() bool                { return i.mode.IsDir() }
func (i catalogInfo) Sys() any                   { return nil }
func (i catalogInfo) Type() fs.FileMode          { return i.mode.Type() }
func (i catalogInfo) Info() (fs.FileInfo, error) { return i, nil }

type catalogMemoryFile struct {
	*bytes.Reader
	info    catalogInfo
	entries []fs.DirEntry
	offset  int
}

func (f *catalogMemoryFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *catalogMemoryFile) Close() error               { return nil }
func (f *catalogMemoryFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if !f.info.IsDir() {
		return nil, fs.ErrInvalid
	}
	if n > 0 && f.offset == len(f.entries) {
		return nil, io.EOF
	}
	end := len(f.entries)
	if n > 0 && f.offset+n < end {
		end = f.offset + n
	}
	result := f.entries[f.offset:end]
	f.offset = end
	return result, nil
}

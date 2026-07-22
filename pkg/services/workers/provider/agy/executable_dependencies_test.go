package agy_test

import (
	"io/fs"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers/provider/agy"
)

type fakeExecutableLocator map[string]string

func (l fakeExecutableLocator) LookPath(name string) (string, error) {
	if path, ok := l[name]; ok {
		return path, nil
	}
	return "", fs.ErrNotExist
}

type fakeExecutableInspector map[string]fs.FileInfo

func (i fakeExecutableInspector) Stat(path string) (fs.FileInfo, error) {
	if info, ok := i[path]; ok {
		return info, nil
	}
	return nil, fs.ErrNotExist
}

type fakeExecutableInfo struct{ directory bool }

func (i fakeExecutableInfo) Name() string       { return "agy" }
func (i fakeExecutableInfo) Size() int64        { return 0 }
func (i fakeExecutableInfo) Mode() fs.FileMode  { return 0o755 }
func (i fakeExecutableInfo) ModTime() time.Time { return time.Time{} }
func (i fakeExecutableInfo) IsDir() bool        { return i.directory }
func (i fakeExecutableInfo) Sys() any           { return nil }

func executableDependencies(
	locations map[string]string,
	existingPaths ...string,
) agy.ExecutableDependencies {
	locator := fakeExecutableLocator(locations)
	inspector := make(fakeExecutableInspector, len(existingPaths))
	for _, path := range existingPaths {
		inspector[path] = fakeExecutableInfo{}
	}
	return agy.ExecutableDependencies{Locator: locator, Inspector: inspector}
}

package gallery

import (
	"context"
	"io"
	"os"
	"runtime"
)

func fixtureInstaller() *Installer {
	return &Installer{OperatingSystem: runtime.GOOS, Environment: func(string) string { return "" }, CacheDirectory: func() (string, error) { return "", os.ErrNotExist },
		ExecutableLocator: galleryMissingLocator{}, MakeDirectories: os.MkdirAll,
		InspectPath: os.Stat, InspectLink: os.Lstat, OpenFile: func(p string) (io.ReadCloser, error) { return os.Open(p) },
		CreateTempFile: func(d, p string) (interface {
			io.Writer
			io.Closer
			Name() string
			Chmod(os.FileMode) error
		}, error) {
			return os.CreateTemp(d, p)
		},
		RenamePath: os.Rename, RemovePath: os.Remove}
}
func downloadLocalAIBinary(ctx context.Context, client localAIHTTPDoer, cache, url string) (string, error) {
	return fixtureInstaller().downloadLocalAIBinary(ctx, client, cache, url)
}

type galleryMissingLocator struct{}

func (galleryMissingLocator) LookPath(string) (string, error) { return "", os.ErrNotExist }

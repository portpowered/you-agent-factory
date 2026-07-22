package work

import (
	"context"
	"io"
	"io/fs"
	"net/http"
)

// ContentHostPlatform identifies the operating-system path convention used
// when resolving local Work content URLs.
type ContentHostPlatform string

// ContentCleanup releases resources created while materializing Work content.
type ContentCleanup func()

// ContentTemporaryFile is the exact temporary-file handle used while
// materializing remote and inline Work content.
type ContentTemporaryFile interface {
	Name() string
	Close() error
}

// ContentInspectPath inspects a local Work content path.
type ContentInspectPath func(string) (fs.FileInfo, error)

// ContentCreateTemporaryFile reserves a materialized Work content path.
type ContentCreateTemporaryFile func(string, string) (ContentTemporaryFile, error)

// ContentRemovePath removes a temporary materialized Work content path.
type ContentRemovePath func(string) error

// ContentWriteFile writes decoded inline Work content.
type ContentWriteFile func(string, []byte, fs.FileMode) error

// ContentOpenFile opens a temporary path for bounded remote-content writes.
type ContentOpenFile func(string) (io.WriteCloser, error)

// ContentHTTPDoer performs the exact outbound HTTP effect used to retrieve
// remote Work content.
type ContentHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// ContentMaterializer resolves a Work content URL to a local path for an
// invocation.
type ContentMaterializer interface {
	MaterializeContentURL(context.Context, string) (string, ContentCleanup, error)
}

// ContentMaterializeFunc adapts a function to ContentMaterializer.
type ContentMaterializeFunc func(context.Context, string) (string, ContentCleanup, error)

func (f ContentMaterializeFunc) MaterializeContentURL(
	ctx context.Context,
	rawURL string,
) (string, ContentCleanup, error) {
	return f(ctx, rawURL)
}

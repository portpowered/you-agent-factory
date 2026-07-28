package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	work "github.com/portpowered/infinite-you/pkg/services/work"
	contentmaterialization "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_materialization"
)

const (
	defaultMaxBytes = 32 << 20 // 32 MiB
	// DefaultHTTPTimeout is the Work-owned outbound retrieval timeout applied
	// by both the request context and Wire's concrete HTTP client.
	DefaultHTTPTimeout  = 30 * time.Second
	defaultMaxRedirects = 3
)

// CleanupFunc releases resources created during materialization (for example temp files).
type CleanupFunc func()

// Options configures MaterializeContentURL behavior.
type Options struct {
	HostPlatform     work.ContentHostPlatform
	MaxBytes         int64
	Timeout          time.Duration
	MaxRedirects     int
	AllowPrivateURLs bool
	HTTPDoer         work.ContentHTTPDoer
	TempDir          string
	InspectPath      work.ContentInspectPath
	CreateTempFile   work.ContentCreateTemporaryFile
	RemovePath       work.ContentRemovePath
	WriteFile        work.ContentWriteFile
	OpenFile         work.ContentOpenFile
}

// Service is the concrete Work content materialization service. Its policy is
// fixed at construction so consumers depend only on work.ContentMaterializer.
type Service struct {
	options Options
}

// New constructs a materializer from flat policy values and exact external
// effects. Zero policy values select the package defaults; effects are required.
func New(
	hostPlatform work.ContentHostPlatform,
	maxBytes int64,
	timeout time.Duration,
	maxRedirects int,
	allowPrivateURLs bool,
	httpDoer work.ContentHTTPDoer,
	tempDir string,
	inspectPath work.ContentInspectPath,
	createTempFile work.ContentCreateTemporaryFile,
	removePath work.ContentRemovePath,
	writeFile work.ContentWriteFile,
	openFile work.ContentOpenFile,
) (*Service, error) {
	if strings.TrimSpace(string(hostPlatform)) == "" {
		return nil, fmt.Errorf("construct Work content materializer: host platform is required")
	}
	if httpDoer == nil {
		return nil, fmt.Errorf("construct Work content materializer: HTTP doer is required")
	}
	if inspectPath == nil {
		return nil, fmt.Errorf("construct Work content materializer: inspect path is required")
	}
	if createTempFile == nil {
		return nil, fmt.Errorf("construct Work content materializer: create temporary file is required")
	}
	if removePath == nil {
		return nil, fmt.Errorf("construct Work content materializer: remove path is required")
	}
	if writeFile == nil {
		return nil, fmt.Errorf("construct Work content materializer: write file is required")
	}
	if openFile == nil {
		return nil, fmt.Errorf("construct Work content materializer: open file is required")
	}
	return &Service{options: Options{
		HostPlatform: hostPlatform,
		MaxBytes:     maxBytes, Timeout: timeout, MaxRedirects: maxRedirects,
		AllowPrivateURLs: allowPrivateURLs, HTTPDoer: httpDoer, TempDir: tempDir,
		InspectPath: inspectPath, CreateTempFile: createTempFile, RemovePath: removePath,
		WriteFile: writeFile, OpenFile: openFile,
	}}, nil
}

// MaterializeContentURL implements work.ContentMaterializer.
func (s *Service) MaterializeContentURL(ctx context.Context, rawURL string) (string, work.ContentCleanup, error) {
	var options *Options
	if s != nil {
		options = &s.options
	}
	path, cleanup, err := MaterializeContentURL(ctx, rawURL, options)
	return path, work.ContentCleanup(cleanup), err
}

var _ work.ContentMaterializer = (*Service)(nil)
var _ contentmaterialization.Service = (*Service)(nil)

func (o *Options) maxBytes() int64 {
	if o == nil || o.MaxBytes <= 0 {
		return defaultMaxBytes
	}
	return o.MaxBytes
}

func (o *Options) timeout() time.Duration {
	if o == nil || o.Timeout <= 0 {
		return DefaultHTTPTimeout
	}
	return o.Timeout
}

func (o *Options) maxRedirects() int {
	if o == nil || o.MaxRedirects <= 0 {
		return defaultMaxRedirects
	}
	return o.MaxRedirects
}

func (o *Options) allowPrivateURLs() bool {
	return o != nil && o.AllowPrivateURLs
}

// MaterializeContentURL resolves a content URL to a readable local filesystem path.
// For file:// URLs the underlying path is returned without copy and cleanup is a no-op.
// For http(s) and data URLs a bounded temp file is created; callers must invoke cleanup when done.
func MaterializeContentURL(ctx context.Context, rawURL string, opts *Options) (localPath string, cleanup CleanupFunc, err error) {
	trimmed := strings.TrimSpace(rawURL)
	if err := work.ValidateContentURL(trimmed); err != nil {
		return "", noopCleanup, fmt.Errorf("scheme not supported: %s", trimmed)
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", noopCleanup, fmt.Errorf("scheme not supported: %s", trimmed)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "file":
		if opts == nil || strings.TrimSpace(string(opts.HostPlatform)) == "" {
			return "", noopCleanup, fmt.Errorf("materialize file url: host platform is required")
		}
		return materializeFileURL(trimmed, parsed, opts)
	case "http", "https":
		return materializeRemoteURLWithTimeout(ctx, trimmed, parsed, opts)
	case "data":
		return materializeDataURL(trimmed, parsed, opts)
	default:
		return "", noopCleanup, fmt.Errorf("scheme not supported: %s", trimmed)
	}
}

func noopCleanup() {}

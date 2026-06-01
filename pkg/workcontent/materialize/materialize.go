package materialize

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/workcontent"
)

const (
	defaultMaxBytes     = 32 << 20 // 32 MiB
	defaultTimeout      = 30 * time.Second
	defaultMaxRedirects = 3
)

// CleanupFunc releases resources created during materialization (for example temp files).
type CleanupFunc func()

// Options configures MaterializeContentURL behavior.
type Options struct {
	MaxBytes         int64
	Timeout          time.Duration
	MaxRedirects     int
	AllowPrivateURLs bool
	HTTPClient       *http.Client
	TempDir          string
}

func (o *Options) maxBytes() int64 {
	if o == nil || o.MaxBytes <= 0 {
		return defaultMaxBytes
	}
	return o.MaxBytes
}

func (o *Options) timeout() time.Duration {
	if o == nil || o.Timeout <= 0 {
		return defaultTimeout
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
	if err := workcontent.ValidateContentURL(trimmed); err != nil {
		return "", noopCleanup, fmt.Errorf("scheme not supported: %s", trimmed)
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", noopCleanup, fmt.Errorf("scheme not supported: %s", trimmed)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "file":
		return materializeFileURL(trimmed, parsed)
	case "http", "https":
		return materializeRemoteURLWithTimeout(ctx, trimmed, parsed, opts)
	case "data":
		return materializeDataURL(trimmed, parsed, opts)
	default:
		return "", noopCleanup, fmt.Errorf("scheme not supported: %s", trimmed)
	}
}

func noopCleanup() {}

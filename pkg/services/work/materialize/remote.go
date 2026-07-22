package materialize

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	work "github.com/portpowered/infinite-you/pkg/services/work"
)

func materializeRemoteURL(ctx context.Context, rawURL string, parsed *url.URL, opts *Options) (string, CleanupFunc, error) {
	if err := validateRemoteTarget(ctx, rawURL, parsed, opts.allowPrivateURLs()); err != nil {
		return "", noopCleanup, err
	}

	if opts == nil || opts.HTTPDoer == nil {
		return "", noopCleanup, inaccessibleError(rawURL, "HTTP doer dependency is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", noopCleanup, inaccessibleError(rawURL, err.Error())
	}

	resp, err := opts.HTTPDoer.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", noopCleanup, inaccessibleError(rawURL, contextFailureReason(ctxErr))
		}
		return "", noopCleanup, inaccessibleError(rawURL, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", noopCleanup, inaccessibleError(rawURL, fmt.Sprintf("http %d", resp.StatusCode))
	}

	ext := extensionFromContentType(resp.Header.Get("Content-Type"))
	path, cleanup, err := createTempFile(opts, ext)
	if err != nil {
		return "", noopCleanup, inaccessibleError(rawURL, err.Error())
	}

	maxBytes := opts.maxBytes()
	written, err := copyLimited(path, resp.Body, maxBytes, opts.OpenFile)
	if err != nil {
		cleanup()
		if err == errSizeLimit {
			return "", noopCleanup, inaccessibleError(rawURL, "response exceeds size limit")
		}
		return "", noopCleanup, inaccessibleError(rawURL, err.Error())
	}
	if written == 0 {
		cleanup()
		return "", noopCleanup, inaccessibleError(rawURL, "empty response body")
	}

	return path, cleanup, nil
}

func contextFailureReason(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "timeout"
}

var errSizeLimit = fmt.Errorf("size limit exceeded")

func copyLimited(path string, r io.Reader, maxBytes int64, openFile work.ContentOpenFile) (int64, error) {
	if openFile == nil {
		return 0, fmt.Errorf("open file dependency is required")
	}
	f, err := openFile(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	limited := io.LimitReader(r, maxBytes+1)
	n, err := io.Copy(f, limited)
	if err != nil {
		return n, err
	}
	if n > maxBytes {
		return n, errSizeLimit
	}
	return n, nil
}

// RedirectPolicy returns the Work-owned redirect policy installed on the
// concrete HTTP client selected by Wire.
func RedirectPolicy(maxRedirects int, allowPrivate bool) func(*http.Request, []*http.Request) error {
	if maxRedirects <= 0 {
		maxRedirects = defaultMaxRedirects
	}
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		return validateRemoteTarget(req.Context(), req.URL.String(), req.URL, allowPrivate)
	}
}

func extensionFromContentType(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch ct {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "audio/mpeg":
		return ".mp3"
	case "video/mp4":
		return ".mp4"
	default:
		return ".bin"
	}
}

func withTimeout(ctx context.Context, opts *Options) (context.Context, context.CancelFunc) {
	timeout := DefaultHTTPTimeout
	if opts != nil && opts.Timeout > 0 {
		timeout = opts.Timeout
	}
	return context.WithTimeout(ctx, timeout)
}

// materializeRemoteURLWithTimeout wraps remote fetch with an explicit timeout when ctx has none.
func materializeRemoteURLWithTimeout(parent context.Context, rawURL string, parsed *url.URL, opts *Options) (string, CleanupFunc, error) {
	ctx, cancel := withTimeout(parent, opts)
	defer cancel()
	return materializeRemoteURL(ctx, rawURL, parsed, opts)
}

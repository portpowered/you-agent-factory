package materialize

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func materializeRemoteURL(ctx context.Context, rawURL string, parsed *url.URL, opts *Options) (string, CleanupFunc, error) {
	if err := validateRemoteTarget(ctx, rawURL, parsed, opts.allowPrivateURLs()); err != nil {
		return "", noopCleanup, err
	}

	client := httpClient(opts)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", noopCleanup, inaccessibleError(rawURL, err.Error())
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", noopCleanup, inaccessibleError(rawURL, "timeout")
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
	written, err := copyLimited(path, resp.Body, maxBytes)
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

var errSizeLimit = fmt.Errorf("size limit exceeded")

func copyLimited(path string, r io.Reader, maxBytes int64) (int64, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
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

func httpClient(opts *Options) *http.Client {
	if opts != nil && opts.HTTPClient != nil {
		return opts.HTTPClient
	}
	var o Options
	if opts != nil {
		o = *opts
	}
	timeout := o.timeout()
	maxRedirects := o.maxRedirects()
	allowPrivate := o.allowPrivateURLs()

	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			if err := validateRemoteTarget(req.Context(), req.URL.String(), req.URL, allowPrivate); err != nil {
				return err
			}
			return nil
		},
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
	timeout := defaultTimeout
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

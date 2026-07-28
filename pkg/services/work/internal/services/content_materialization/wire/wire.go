// Package wire constructs the Work content_materialization nested subservice from
// exact injected effect ports.
package wire

import (
	"fmt"
	"net/http"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	contentmaterialization "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_materialization"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_materialization/internal/service"
)

// DefaultHTTPTimeout is the Work-owned outbound retrieval timeout applied by
// both the request context and Wire's concrete HTTP client.
const DefaultHTTPTimeout = internalservice.DefaultHTTPTimeout

// RedirectPolicy returns the Work-owned redirect policy installed on the
// concrete HTTP client selected by Wire.
func RedirectPolicy(maxRedirects int, allowPrivate bool) func(*http.Request, []*http.Request) error {
	return internalservice.RedirectPolicy(maxRedirects, allowPrivate)
}

// NewService constructs the nested content_materialization capability.
func NewService(
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
) (contentmaterialization.Service, error) {
	service, err := internalservice.New(
		hostPlatform,
		maxBytes,
		timeout,
		maxRedirects,
		allowPrivateURLs,
		httpDoer,
		tempDir,
		inspectPath,
		createTempFile,
		removePath,
		writeFile,
		openFile,
	)
	if err != nil {
		return nil, fmt.Errorf("construct Work content materialization: %w", err)
	}
	return service, nil
}

var _ contentmaterialization.Service = (*internalservice.Service)(nil)

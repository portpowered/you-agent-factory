// Package wire is the Work service composition boundary. Application Wire uses
// these providers without importing Work internal implementation packages.
package wire

import (
	"net/http"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	contentstagingwire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_staging/wire"
	contentmaterializationwire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_materialization/wire"
)

// DefaultContentMaterializationHTTPTimeout is the Work-owned outbound retrieval
// timeout applied by both the request context and application Wire's HTTP client.
const DefaultContentMaterializationHTTPTimeout = contentmaterializationwire.DefaultHTTPTimeout

// ContentMaterializationRedirectPolicy returns the Work-owned redirect policy
// installed on the concrete HTTP client selected by application Wire.
func ContentMaterializationRedirectPolicy(maxRedirects int, allowPrivate bool) func(*http.Request, []*http.Request) error {
	return contentmaterializationwire.RedirectPolicy(maxRedirects, allowPrivate)
}

// NewContentStagingService constructs the nested content_staging capability and
// returns it as the published Work ContentStagingService role.
func NewContentStagingService(
	filesystem work.ContentStagingFileSystem,
	random work.ContentStagingRandom,
	clock work.ContentStagingClock,
	ttl time.Duration,
) (work.ContentStagingService, error) {
	return contentstagingwire.NewService(filesystem, random, clock, ttl)
}

// NewContentMaterializationService constructs the nested content_materialization
// capability and returns it as the published Work ContentMaterializer role.
func NewContentMaterializationService(
	hostPlatform work.ContentHostPlatform,
	httpDoer work.ContentHTTPDoer,
	inspectPath work.ContentInspectPath,
	createTempFile work.ContentCreateTemporaryFile,
	removePath work.ContentRemovePath,
	writeFile work.ContentWriteFile,
	openFile work.ContentOpenFile,
) (work.ContentMaterializer, error) {
	return contentmaterializationwire.NewService(
		hostPlatform, 0, 0, 0, false, httpDoer, "",
		inspectPath, createTempFile, removePath, writeFile, openFile,
	)
}

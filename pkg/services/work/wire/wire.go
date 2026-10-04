// Package wire is the Work service composition boundary.
//
// Wire exposes focused, inert providers for the completed Work root and its
// preparation, state-access and content capabilities. Canonical application
// composition selects each leaf once without importing Work-private packages.
package wire

import (
	"net/http"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal"
	contentmaterializationwire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_materialization/wire"
	contentstagingwire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_staging/wire"
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

// NewRuntimeService constructs a session-scoped Work root from application-wired
// content collaborators. Application composition supplies shared content staging
// and materialization services so runtime opening and peer edges observe the same
// instances.
func NewRuntimeService(
	runtimes work.RuntimeResolver,
	readSubmittedFile work.SubmittedFileReader,
	inspectSubmittedFile work.SubmittedFilePathInspector,
	contentStaging work.ContentStagingService,
	contentMaterializer work.ContentMaterializer,
	stateAccess StateAccess,
	preparation work.RequestPreparationService,
	invocationPreparation work.InvocationInputPreparation,
) work.Service {
	return internalservice.NewService(
		runtimes,
		readSubmittedFile,
		inspectSubmittedFile,
		contentStaging,
		contentMaterializer,
		stateAccess, preparation, invocationPreparation,
	)
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

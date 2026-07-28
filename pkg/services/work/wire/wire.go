// Package wire is the Work service composition boundary.
//
// Wire performs construction only, returns the singular work.Service root
// interface, and starts no lifecycle components. Parent-private
// content_staging, content_materialization, and state_access owner wiring stays
// inside the owner service assembly path; peers depend on Service rather than
// owner internals or construction ports. Application Wire may continue using
// nested content helper constructors without importing Work internal packages.
package wire

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal/service"
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

// NewService constructs an inert Work root from construction and process-edge
// ports. It composes the accepted root through parent-private content_staging,
// content_materialization, and state_access owners without publishing owner
// types on the returned peer surface.
func NewService(
	runtimes work.RuntimeResolver,
	filesystem work.ContentStagingFileSystem,
	random work.ContentStagingRandom,
	clock work.ContentStagingClock,
	stagingTTL time.Duration,
	hostPlatform work.ContentHostPlatform,
	httpDoer work.ContentHTTPDoer,
	inspectPath work.ContentInspectPath,
	createTempFile work.ContentCreateTemporaryFile,
	removePath work.ContentRemovePath,
	writeFile work.ContentWriteFile,
	openFile work.ContentOpenFile,
) (work.Service, error) {
	if err := validateNewServiceInputs(
		runtimes,
		filesystem,
		random,
		clock,
		hostPlatform,
		httpDoer,
		inspectPath,
		createTempFile,
		removePath,
		writeFile,
		openFile,
	); err != nil {
		return nil, err
	}
	contentStaging, err := contentstagingwire.NewService(filesystem, random, clock, stagingTTL)
	if err != nil {
		return nil, err
	}
	// Wire validation covers nested materialization construction preconditions.
	contentMaterializer, _ := contentmaterializationwire.NewService(
		hostPlatform,
		0,
		0,
		0,
		false,
		httpDoer,
		"",
		inspectPath,
		createTempFile,
		removePath,
		writeFile,
		openFile,
	)
	service := internalservice.NewService(runtimes, nil, contentStaging, contentMaterializer)
	return service, nil
}

// NewRuntimeService constructs a session-scoped Work root from application-wired
// content collaborators. Application composition supplies shared content staging
// and materialization services so runtime opening and peer edges observe the same
// instances.
func NewRuntimeService(
	runtimes work.RuntimeResolver,
	readSubmittedFile work.SubmittedFileReader,
	contentStaging work.ContentStagingService,
	contentMaterializer work.ContentMaterializer,
) work.Service {
	return internalservice.NewService(
		runtimes,
		readSubmittedFile,
		contentStaging,
		contentMaterializer,
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

func validateNewServiceInputs(
	runtimes work.RuntimeResolver,
	filesystem work.ContentStagingFileSystem,
	random work.ContentStagingRandom,
	clock work.ContentStagingClock,
	hostPlatform work.ContentHostPlatform,
	httpDoer work.ContentHTTPDoer,
	inspectPath work.ContentInspectPath,
	createTempFile work.ContentCreateTemporaryFile,
	removePath work.ContentRemovePath,
	writeFile work.ContentWriteFile,
	openFile work.ContentOpenFile,
) error {
	if runtimes == nil {
		return fmt.Errorf("construct Work: runtime resolver is required")
	}
	if filesystem == nil {
		return fmt.Errorf("construct Work: content staging filesystem is required")
	}
	if random == nil {
		return fmt.Errorf("construct Work: content staging random is required")
	}
	if clock == nil {
		return fmt.Errorf("construct Work: content staging clock is required")
	}
	if strings.TrimSpace(string(hostPlatform)) == "" {
		return fmt.Errorf("construct Work: content host platform is required")
	}
	if httpDoer == nil {
		return fmt.Errorf("construct Work: HTTP doer is required")
	}
	if inspectPath == nil {
		return fmt.Errorf("construct Work: inspect path is required")
	}
	if createTempFile == nil {
		return fmt.Errorf("construct Work: create temporary file is required")
	}
	if removePath == nil {
		return fmt.Errorf("construct Work: remove path is required")
	}
	if writeFile == nil {
		return fmt.Errorf("construct Work: write file is required")
	}
	if openFile == nil {
		return fmt.Errorf("construct Work: open file is required")
	}
	return nil
}

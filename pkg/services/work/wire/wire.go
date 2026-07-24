// Package wire is the Work service composition boundary. Application Wire uses
// these providers without importing Work internal implementation packages.
package wire

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	contentstagingwire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_staging/wire"
)

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

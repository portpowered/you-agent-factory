// Package wire constructs the Work content_staging nested subservice from exact
// injected effect ports.
package wire

import (
	"fmt"
	"time"

	contentstaging "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_staging"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_staging/internal/service"
)

// NewService constructs the nested content_staging capability.
func NewService(
	filesystem contentstaging.FileSystem,
	random contentstaging.Random,
	clock contentstaging.Clock,
	ttl time.Duration,
) (contentstaging.Service, error) {
	service, err := internalservice.New(filesystem, random, clock, ttl)
	if err != nil {
		return nil, fmt.Errorf("construct Work content staging: %w", err)
	}
	return service, nil
}

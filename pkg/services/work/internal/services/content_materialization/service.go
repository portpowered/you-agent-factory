// Package content_materialization owns Work content URL resolution, SSRF and
// size policy, per-dispatch cache, cleanup, and cancellation behind the
// published CTR-WORK materialization seam. Peers consume Work root contracts;
// this package is the parent-private nested owner for the materialization effect.
package content_materialization

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Service is the singular content_materialization subservice contract.
type Service interface {
	MaterializeContentURL(context.Context, string) (string, work.ContentCleanup, error)
}

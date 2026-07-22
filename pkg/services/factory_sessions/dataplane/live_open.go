package dataplane

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// LiveOpener delegates live session startup to the runtime host seam.
type LiveOpener struct {
	host LiveOpenHost
}

// NewLiveOpener constructs a live open collaborator.
func NewLiveOpener(host LiveOpenHost) *LiveOpener {
	return &LiveOpener{host: host}
}

// OpenForTarget starts one live session for the selected target.
func (o *LiveOpener) OpenForTarget(ctx context.Context, target factorysessions.Target) (string, error) {
	if o == nil || o.host == nil {
		return "", fmt.Errorf("live session dataplane host is required")
	}
	return o.host.OpenLiveSessionForTarget(ctx, target)
}

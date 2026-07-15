package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/work"
)

// WorkRequestSubmitter submits parsed poller or cron work requests into the runtime.
type WorkRequestSubmitter func(context.Context, work.WorkRequest) error

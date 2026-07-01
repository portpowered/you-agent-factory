package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// WorkRequestSubmitter submits parsed poller or cron work requests into the runtime.
type WorkRequestSubmitter func(context.Context, interfaces.WorkRequest) error

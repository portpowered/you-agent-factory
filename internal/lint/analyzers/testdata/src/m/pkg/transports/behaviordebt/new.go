package behaviordebt

import (
	"context"
	"time"
)

func distinct(ctx context.Context) { _, _ = context.WithTimeout(ctx, time.Second) } // want `transport-lifecycle:.*context.WithTimeout`

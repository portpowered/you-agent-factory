//go:build integration

package behaviortagged

import "context"

func run(ctx context.Context) { _, _ = context.WithCancel(ctx) }

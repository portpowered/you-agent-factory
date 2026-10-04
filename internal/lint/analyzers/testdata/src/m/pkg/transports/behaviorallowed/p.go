package behaviorallowed

import (
	"context"
	"io"
	models "m/pkg/services/models"
	workers "m/pkg/services/workers"
	"time"
)

type preparation interface{ PrepareStart() }
type operation interface{ Execute(context.Context) }

func Run(ctx context.Context, out io.Writer, prepare preparation, op operation, input string) {
	prepare.PrepareStart()
	_, _ = out.Write([]byte(input))
	op.Execute(ctx)
	models.IsMissing()
	workers.NormalizeProviderExecutionError()
	_ = time.Duration(1)
}

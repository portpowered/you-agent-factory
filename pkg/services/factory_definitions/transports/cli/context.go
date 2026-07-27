package cli

import (
	"context"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
)

func workingDirectoryFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	return startupcli.WorkingDirectory(ctx)
}

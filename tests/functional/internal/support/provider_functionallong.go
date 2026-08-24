//go:build functionallong

package support

import (
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

func ProviderErrorCommandResult(t *testing.T, name string) platformprocess.CommandResult {
	t.Helper()
	return providerErrorCommandResult(t, name)
}

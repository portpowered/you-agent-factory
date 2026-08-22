package runtimeopening

import (
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestOperatorConfigPathRequiresExplicitProcessHome(t *testing.T) {
	t.Parallel()

	_, err := operatorConfigPath(factorysessions.SessionRuntimeOpeningRequest{})
	if err == nil || !strings.Contains(err.Error(), "operator config home is required") {
		t.Fatalf("operatorConfigPath() error = %v, want required process home", err)
	}
}

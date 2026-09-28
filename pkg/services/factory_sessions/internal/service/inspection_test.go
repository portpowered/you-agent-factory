package service

import (
	"testing"

	legacyservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
)

func TestSessionInspectionServiceUsesProcessAssemblyOwner(t *testing.T) {
	owner := &legacyservice.Service{}
	root := &Root{Assembly: &legacyservice.Assembly{Service: owner}}
	if got := root.SessionInspectionService(); got != owner {
		t.Fatalf("SessionInspectionService() = %#v, want process assembly owner %#v", got, owner)
	}
}

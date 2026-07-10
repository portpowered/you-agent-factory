package initializer

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/runtimehost"
)

func TestComposeSessionAPISurfaceRejectsUnavailableCollaborator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		services *Services
		host     *SessionRuntimeHost
		wantRole string
	}{
		{
			name:     "session host",
			services: &Services{},
			wantRole: "session collaborator is required",
		},
		{
			name:     "model service",
			services: &Services{},
			host:     &SessionRuntimeHost{host: &runtimehost.Host{}},
			wantRole: "model collaborator is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := composeSessionAPISurface(tt.services, tt.host)
			if err == nil || !strings.Contains(err.Error(), tt.wantRole) {
				t.Fatalf("composeSessionAPISurface() error = %v, want role %q", err, tt.wantRole)
			}
		})
	}
}

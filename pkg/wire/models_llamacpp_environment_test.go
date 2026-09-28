package wire

import (
	"context"
	"errors"
	"strings"
	"testing"

	managedchild "github.com/portpowered/infinite-you/pkg/platform/process/managedchild"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/wire/internal/managedbackend"
)

func TestModelsProcessLauncherRemovesInheritedLlamaCPPLauncherSwitch(t *testing.T) {
	t.Setenv("LLAMACPP_GRPC_SERVERS", "1")
	var childEnvironment []string
	launcher := modelsProcessLauncher{
		resolveLaunch: func(context.Context, serviceedges.HostProcessStartSpec) (managedbackend.ManagedBackendLaunch, error) {
			return managedbackend.ManagedBackendLaunch{
				Command: "llama-cpp-grpc", Endpoint: "grpc://127.0.0.1:1",
				Env: []string{"LD_LIBRARY_PATH=/managed/lib"}, Cleanup: func() error { return nil },
			}, nil
		},
		startProcess: func(_ context.Context, spec managedchild.Spec) (*managedchild.Process, error) {
			childEnvironment = spec.Env
			return nil, errors.New("controlled stop before launch")
		},
	}
	_, _ = launcher.Start(context.Background(), serviceedges.HostProcessStartSpec{Backend: "localai-llamacpp"})
	if len(childEnvironment) == 0 {
		t.Fatal("managed child environment was not captured")
	}
	for _, entry := range childEnvironment {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, "LLAMACPP_GRPC_SERVERS") {
			t.Fatalf("managed llama-cpp child inherited launcher switch: %q", entry)
		}
	}
}

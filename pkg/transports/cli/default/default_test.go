package defaultcmd

import (
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

func TestRunConfigsPreserveExplicitAndOutOfBoxModes(t *testing.T) {
	defaults := runcli.RunConfig{}
	explicit := ExplicitRunConfig(defaults)
	if explicit.Dir != FactoryDir || explicit.Port != FactoryPort || !explicit.AutoPort {
		t.Fatalf("ExplicitRunConfig() = %+v", explicit)
	}
	if explicit.Continuously || explicit.Bootstrap || explicit.OpenDashboard {
		t.Fatalf("ExplicitRunConfig() enabled default-startup lifecycle: %+v", explicit)
	}

	outOfBox := OOTBRunConfig(defaults)
	if outOfBox.Dir != FactoryDir || outOfBox.Port != FactoryPort || !outOfBox.AutoPort ||
		!outOfBox.Continuously || !outOfBox.Bootstrap || !outOfBox.OpenDashboard {
		t.Fatalf("OOTBRunConfig() = %+v", outOfBox)
	}
}

func TestServerRunConfigUsesOwnedNonBootstrappingLifecycle(t *testing.T) {
	defaults := runcli.RunConfig{}
	server := ServerRunConfig(defaults)
	if server.Dir != FactoryDir || server.Port != FactoryPort || !server.AutoPort ||
		!server.Continuously || server.Bootstrap || !server.OpenDashboard {
		t.Fatalf("ServerRunConfig() = %+v", server)
	}
}

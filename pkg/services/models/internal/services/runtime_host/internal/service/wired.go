package service

import (
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
	leaseswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases/wire"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// NewWired constructs Runtime Host with a host-backed SlotFactsProvider bound to
// the nested leases owner. Construction remains inert and does not launch
// subprocesses or start application lifecycle.
func NewWired(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	processLauncher modelseffects.HostProcessLauncher,
	hostHTTP modelseffects.HostHTTPDoer,
	hostClock modelseffects.HostClock,
	hostLogger modelseffects.HostDiagnosticLogger,
	hostMetrics modelseffects.HostMetricsRecorder,
) (runtimehost.Service, error) {
	adapter := &slotFactsAdapter{}
	leases, err := leaseswire.NewService(hostClock, adapter)
	if err != nil {
		return nil, err
	}
	host := New(
		scopes,
		assets,
		leases,
		processLauncher,
		hostHTTP,
		hostClock,
		hostLogger,
		hostMetrics,
	).(*service)
	adapter.host = host
	return host, nil
}

// LeasesOwner returns the nested leases capability for focused integration tests.
func LeasesOwner(host runtimehost.Service) hostleases.Service {
	return host.(*service).leases
}

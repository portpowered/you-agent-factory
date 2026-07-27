package service

import (
	models "github.com/portpowered/infinite-you/pkg/services/models"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
	leaseswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases/wire"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// NewWired constructs Runtime Host with a host-backed SlotFactsProvider bound to
// the nested leases owner. Construction remains inert and does not launch
// subprocesses or start application lifecycle.
func NewWired(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	processLauncher models.HostProcessLauncher,
	hostHTTP models.HostHTTPDoer,
	hostClock models.HostClock,
	hostLogger models.HostDiagnosticLogger,
	hostMetrics models.HostMetricsRecorder,
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

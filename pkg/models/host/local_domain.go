package modelhost

import (
	"fmt"

	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
)

// LocalDomain contains the model-owned collaborators shared by catalog,
// managed-runtime, and worker invocation behavior.
type LocalDomain struct {
	Resources      *localmodels.ResourceLimiter
	Assets         localmodels.AssetPuller
	Runtime        localmodels.Runtime
	Manager        *localmodels.Manager
	Host           Host
	LeaseExecution *LeaseExecution
}

// LocalDomainDependencies carries application-supplied model edges. Asset,
// runtime, process, source, and host omissions select inert package-owned
// production defaults; Hooks and Diagnostics are optional instrumentation.
type LocalDomainDependencies struct {
	CacheDir        string
	AssetPuller     localmodels.AssetPuller
	Runtime         localmodels.Runtime
	ProcessLauncher ProcessLauncher
	Host            Host
	Hooks           localmodels.Hooks
	Diagnostics     Diagnostics
}

// NewLocalDomain constructs one validated local-model domain without resolving
// assets, launching a model process, or starting application lifecycle.
func NewLocalDomain(deps LocalDomainDependencies) (LocalDomain, error) {
	assets := deps.AssetPuller
	if isNilDependency(assets) {
		assets = localmodels.NewAssetPuller(deps.CacheDir)
	}
	runtime := deps.Runtime
	if isNilDependency(runtime) {
		runtime = localmodels.DefaultRuntime()
	}
	manager, err := localmodels.NewManagedRuntime(localmodels.ManagedRuntimeDependencies{
		AssetPuller: assets,
		Runtime:     runtime,
		Hooks:       deps.Hooks,
	})
	if err != nil {
		return LocalDomain{}, fmt.Errorf("construct managed local runtime: %w", err)
	}

	host := deps.Host
	if isNilDependency(host) {
		launcher := deps.ProcessLauncher
		if isNilDependency(launcher) {
			launcher = DefaultProcessLauncher()
		}
		gateway := NewLocalAssetGateway(assets)
		host, err = NewHost(Dependencies{
			AssetPuller:     gateway,
			CacheInspector:  gateway,
			ProcessLauncher: launcher,
			Options: Options{
				Diagnostics: deps.Diagnostics,
			},
		})
		if err != nil {
			return LocalDomain{}, fmt.Errorf("construct model host: %w", err)
		}
	}

	domain := LocalDomain{
		Resources: localmodels.NewResourceLimiter(deps.Hooks),
		Assets:    assets,
		Runtime:   runtime,
		Manager:   manager,
		Host:      host,
	}
	domain.LeaseExecution = NewLeaseExecution(host, assets, runtime, deps.Hooks)
	return domain, nil
}

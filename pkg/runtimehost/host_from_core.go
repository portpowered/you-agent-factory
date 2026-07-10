package runtimehost

// HostShell is the pre-factorysave Host assembly product for wire composition.
type HostShell struct {
	Host *Host
}

// NewHostFromCore wraps a built core in the runtime/session host returned to
// transports and compatibility callers.
func NewHostFromCore(core *Core) *Host {
	if core == nil {
		return nil
	}
	host := &Host{
		core:             core,
		factoryRootDir:   core.FactoryRootDir(),
		sessions:         core.Sessions(),
		hostedWorkers:    core.HostedWorkers(),
		policy:           CoordinatorPolicyFromConfig(core.cfg),
		startupBundle:    core.StartupBundle(),
		cfg:              core.cfg,
		modelAssets:      core.modelAssets,
		baseLogger:       core.BaseLogger(),
		logger:           core.Logger(),
		clock:            core.Clock(),
		runtimeBuild:     core.RuntimeBuild(),
		workersScheduler: core.WorkersScheduler(),
	}
	host.coordinator = newCoordinator(host)
	host.definitions = newFactoryDefinitionService(host)
	host.modelService = wireModelServiceCollaborator(host, core.cfg)
	return host
}

// ComposeCollaboratorSnapshot reports initialized collaborators for equivalence tests.
func (h *Host) ComposeCollaboratorSnapshot() ComposeCollaboratorSnapshot {
	if h == nil {
		return ComposeCollaboratorSnapshot{}
	}
	snapshot := ComposeCollaboratorSnapshot{
		ModelServiceInitialized: h.modelService != nil,
		FactorySaveInitialized:  h.factorySave != nil,
		DefinitionsInitialized:  h.definitions != nil,
	}
	if h.core != nil {
		coreSnapshot := h.core.ComposeCollaboratorSnapshot()
		coreSnapshot.ModelServiceInitialized = snapshot.ModelServiceInitialized
		coreSnapshot.FactorySaveInitialized = snapshot.FactorySaveInitialized
		coreSnapshot.DefinitionsInitialized = snapshot.DefinitionsInitialized
		return coreSnapshot
	}
	bundle := h.currentRuntimeBundle()
	snapshot.SessionsInitialized = h.sessions != nil
	snapshot.RuntimeBuildInitialized = h.runtimeBuild != nil
	snapshot.WorkersSchedulerInitialized = h.workersScheduler != nil
	snapshot.ModelAssetsInitialized = h.modelAssets != nil
	snapshot.HostedWorkersLoggerReady = h.hostedWorkers.Logger != nil
	if bundle != nil {
		snapshot.BundleModelResources = bundle.ModelResources != nil
		snapshot.BundleLocalModels = bundle.LocalModels != nil
	}
	return snapshot
}

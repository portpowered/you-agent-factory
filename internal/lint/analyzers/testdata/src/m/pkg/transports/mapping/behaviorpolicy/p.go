package behaviorpolicy

import (
	platform "m/pkg/platform/transportfixture"
	d "m/pkg/services/factory_definitions"
	sessions "m/pkg/services/factory_sessions"
	models "m/pkg/services/models"
	w "m/pkg/services/work"
	workers "m/pkg/services/workers"
)

type preparation interface{ PrepareStart() }
type normalizer interface{ NormalizeWorkRequest() }

func run(v d.Validator, prepare preparation, normalize normalizer) {
	v.ValidateBlockingLoad()                  // want `transport-validation-orchestration:.*ValidateBlockingLoad`
	prepare.PrepareStart()                    // want `transport-factory-session-preparation:.*PrepareStart`
	normalize.NormalizeWorkRequest()          // want `transport-work-request-normalization:.*NormalizeWorkRequest`
	d.MapDir()                                // want `transport-named-factory-path-policy:.*pkg/services/factory_definitions.MapDir`
	d.CompatibleWorkerWorkstationBehavior()   // want `transport-validation-orchestration:.*CompatibleWorkerWorkstationBehavior`
	w.NormalizeArguments()                    // want `transport-work-request-preparation:.*pkg/services/work.NormalizeArguments`
	workers.LoadMockWorkersConfig()           // want `transport-workers-config-loading:.*LoadMockWorkersConfig`
	workers.NormalizeProviderExecutionError() // want `transport-workers-failure-policy:.*NormalizeProviderExecutionError`
	models.IsMissing()                        // want `transport-models-readiness-policy:.*IsMissing`
	sessions.NormalizeStartRequest()          // want `transport-service-normalization:.*NormalizeStartRequest`
	sessions.ProjectRuntimeContract()         // want `transport-domain-policy:.*ProjectRuntimeContract`
	platform.BuildLogger()                    // want `transport-platform-selection:.*BuildLogger`
}
func Forward(service sessions.Service) int { return service.Execute() } // want `transport-service-forwarder:.*Forward->pkg/services/factory_sessions.Execute`
type Unrelated struct{}

func (Unrelated) ValidateBlockingLoad() {}
func unrelated(u Unrelated)             { u.ValidateBlockingLoad() }

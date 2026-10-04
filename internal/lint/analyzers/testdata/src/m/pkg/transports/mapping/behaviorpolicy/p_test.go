package behaviorpolicy

import (
	"context"
	d "m/pkg/services/factory_definitions"
	sessions "m/pkg/services/factory_sessions"
	models "m/pkg/services/models"
	w "m/pkg/services/work"
	workers "m/pkg/services/workers"
)

func testPolicy(prepare preparation, normal normalizer) {
	prepare.PrepareStart()                    // want `transport-factory-session-preparation-test:.*PrepareStart`
	normal.NormalizeWorkRequest()             // want `transport-work-request-normalization-test:.*NormalizeWorkRequest`
	d.MapDir()                                // want `transport-named-factory-path-policy-test:.*MapDir`
	w.NormalizeArguments()                    // want `transport-work-request-preparation-test:.*NormalizeArguments`
	workers.LoadMockWorkersConfig()           // want `transport-workers-config-loading-test:.*LoadMockWorkersConfig`
	workers.NormalizeProviderExecutionError() // want `transport-workers-failure-policy-test:.*NormalizeProviderExecutionError`
	models.IsMissing()                        // want `transport-models-readiness-policy-test:.*IsMissing`
	sessions.NormalizeStartRequest()          // want `transport-service-normalization-test:.*NormalizeStartRequest`
	_ = context.Background()
	sessions.ProjectRuntimeContract()
}

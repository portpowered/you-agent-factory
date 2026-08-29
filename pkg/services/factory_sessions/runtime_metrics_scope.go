package factorysessions

import "context"

// RuntimeMetricsScope is the Factory Sessions-owned identity set used to
// select one live or resumed runtime metrics lineage. The requested ID is the
// public selector; retained IDs are canonical persisted identities only.
type RuntimeMetricsScope struct {
	RequestedFactorySessionID string
	RetainedFactorySessionIDs []string
}

// RuntimeMetricsScopeResolver resolves a public Factory Session selector to
// the exact retained canonical identities that base metrics and Costs must
// query. The resolver owns selector compatibility; consumers must not rebuild
// the set from projection fields or filesystem artifacts.
type RuntimeMetricsScopeResolver interface {
	ResolveRuntimeMetricsScope(context.Context, string) (RuntimeMetricsScope, error)
}

// RuntimeMetricsScopeSessionReader is the narrow Factory Sessions read
// capability needed to resolve one metrics scope. It keeps the scope
// operation independent of the aggregate Service contract.
type RuntimeMetricsScopeSessionReader interface {
	GetFactorySession(context.Context, string) (SessionProjection, error)
}

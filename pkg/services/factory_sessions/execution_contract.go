package factorysessions

// Durable Factory Session execution contracts are owned by this service root.
// Nested internal/execution implements runtime behavior and aliases these types;
// peers must compile against the root vocabulary rather than nested packages.
//
// Published naming notes:
//   - ExecutionService is the durable execution facet embedded by Service
//   - ExecutionValidationError names ValidationError for peer-facing clarity
//   - ErrDurableSessionNotFound is the durable missing-session sentinel
//   - ErrExecutionServiceNotConfigured names the durable service configuration failure
//   - ExecutionFactoryEventConsumer names FactoryEventConsumer for durable sync observation
//
// Helper functions ApplySessionListScope, EvaluateLifecycleControl,
// MaterializeEventReadStream, and related lifecycle/listing helpers are also
// root-owned so the root does not re-export nested implementation functions.

// ContractFixtureCatalogRelativePath is the repository-relative durable
// session fixture catalog used by explicit fake execution entrypoints.
const ContractFixtureCatalogRelativePath = "pkg/transports/http/testdata/durable-session-contract-fixtures.json"

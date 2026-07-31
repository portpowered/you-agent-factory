package workers

// Workers-owned peer execution and diagnostics vocabulary. Cross-service
// consumers import these types from pkg/services/workers rather than treating
// worker execution surfaces as Factory Definitions-owned peer contracts.
//
// Primary surfaces:
//   - execution_contracts.go: outcomes, failures, workstation results, provider
//     inference requests, dispatch payloads, and clone helpers.
//   - safe_diagnostics.go and safe_diagnostics_codec.go: dashboard-safe
//     diagnostics projections and round-trip helpers.
//   - execution_requests.go: runner selection and subprocess execution requests.
//   - interfaces.go: Provider and CommandRunner ports.

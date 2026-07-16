package sessionparity

// CapturedObservations contains equivalent Factory Session capture bundles from
// each customer interface. Every bundle contains the separate session,
// dispatch, artifact, result, and event reads used by the parity projection.
type CapturedObservations struct {
	REST    []byte
	CLIJSON []byte
	MCP     []byte
}

// TerminalSuccessObservations returns deterministic real-shape customer reads
// for one completed Factory Session.
func TerminalSuccessObservations() CapturedObservations {
	return capturedObservations(scenarioCapture{
		"dur-sess-terminal-success", successSession, successDispatches, successArtifacts, successResult, successEvents,
	})
}

// TerminalFailureObservations returns deterministic real-shape customer reads
// for one inspectable terminal Factory Session failure.
func TerminalFailureObservations() CapturedObservations {
	return capturedObservations(scenarioCapture{
		"dur-sess-terminal-failure", failureSession, failureDispatches, failureArtifacts, failureResult, failureEvents,
	})
}

type scenarioCapture struct {
	SessionID                                      string
	Session, Dispatches, Artifacts, Result, Events string
}

func capturedObservations(capture scenarioCapture) CapturedObservations {
	direct := []byte(`{"session":` + capture.Session + `,"dispatches":` + capture.Dispatches +
		`,"artifacts":` + capture.Artifacts + `,"result":` + capture.Result + `,"events":` + capture.Events + `}`)
	eventResult := `{"sessionId":"` + capture.SessionID + `","events":` + capture.Events + `}`
	mcp := []byte(`{"session":{"jsonrpc":"2.0","id":"fixture-session","result":` + capture.Session + `},` +
		`"dispatches":{"jsonrpc":"2.0","id":"fixture-dispatches","result":` + capture.Dispatches + `},` +
		`"artifacts":{"jsonrpc":"2.0","id":"fixture-artifacts","result":` + capture.Artifacts + `},` +
		`"result":{"jsonrpc":"2.0","id":"fixture-result","result":` + capture.Result + `},` +
		`"events":{"jsonrpc":"2.0","id":"fixture-events","result":` + eventResult + `}}`)
	return CapturedObservations{
		REST: append([]byte(nil), direct...), CLIJSON: append([]byte(nil), direct...), MCP: mcp,
	}
}

const successSession = `{
  "sessionId":"dur-sess-terminal-success","status":"SUCCEEDED","phase":"completed",
  "orchestratorKind":"JAVASCRIPT","resolvedSource":{"kind":"WORKFLOW_NAME","sourceHash":"sha256:success-source"},
  "sourceHash":"sha256:success-source","requestedPolicy":{"policyHash":"sha256:success-requested-policy"},
  "effectivePolicyHash":"sha256:success-effective-policy",
  "progress":{"totalDispatches":2,"completedDispatches":2,"failedDispatches":0,"inFlightDispatches":0}
}`

const successDispatches = `{"sessionId":"dur-sess-terminal-success","dispatches":[
  {"id":"dispatch-success-1","status":"COMPLETED","dispatchKind":"JAVASCRIPT_TASK"},
  {"id":"dispatch-success-2","status":"COMPLETED","dispatchKind":"JAVASCRIPT_TASK"}
]}`

const successArtifacts = `{"sessionId":"dur-sess-terminal-success","artifacts":[
  {"id":"artifact-success-result","kind":"FINAL_RESULT","visibility":"PUBLIC"}
]}`

const successResult = `{
  "sessionId":"dur-sess-terminal-success","resultStatus":"FINAL","sessionStatus":"SUCCEEDED","mode":"final",
  "primaryResult":[{"type":"text","text":"fixture success"}]
}`

const successEvents = `[
  {"id":"success-cursor-100","type":"FACTORY_SESSION_STARTED","schemaVersion":"agent-factory.event.v1","context":{"sequence":100,"sessionId":"dur-sess-terminal-success","tick":1,"eventTime":"2026-07-16T00:00:00Z"},"payload":{}},
  {"id":"success-cursor-101","type":"DISPATCH_RESPONSE","schemaVersion":"agent-factory.event.v1","context":{"sequence":101,"sessionId":"dur-sess-terminal-success","tick":2,"eventTime":"2026-07-16T00:00:01Z"},"payload":{}},
  {"id":"success-cursor-102","type":"SESSION_RESULT_UPDATED","schemaVersion":"agent-factory.event.v1","context":{"sequence":102,"sessionId":"dur-sess-terminal-success","tick":3,"eventTime":"2026-07-16T00:00:02Z"},"payload":{}}
]`

const failureSession = `{
  "sessionId":"dur-sess-terminal-failure","status":"FAILED","phase":"failed",
  "orchestratorKind":"JAVASCRIPT","resolvedSource":{"kind":"WORKFLOW_NAME","sourceHash":"sha256:failure-source"},
  "sourceHash":"sha256:failure-source","requestedPolicy":{"policyHash":"sha256:failure-requested-policy"},
  "effectivePolicyHash":"sha256:failure-effective-policy",
  "progress":{"totalDispatches":2,"completedDispatches":1,"failedDispatches":1,"inFlightDispatches":0},
  "failureDetail":{"reason":"WORKER_FAILED","message":"fixture worker failed"}
}`

const failureDispatches = `{"sessionId":"dur-sess-terminal-failure","dispatches":[
  {"id":"dispatch-failure-1","status":"COMPLETED","dispatchKind":"JAVASCRIPT_TASK"},
  {"id":"dispatch-failure-2","status":"FAILED","dispatchKind":"JAVASCRIPT_TASK","failureDetail":{"reason":"WORKER_FAILED","message":"fixture worker failed"}}
]}`

const failureArtifacts = `{"sessionId":"dur-sess-terminal-failure","artifacts":[
  {"id":"artifact-failure-diagnostic","kind":"DIAGNOSTIC","visibility":"SESSION"}
]}`

const failureResult = `{
  "sessionId":"dur-sess-terminal-failure","resultStatus":"UNAVAILABLE","sessionStatus":"FAILED","mode":"final",
  "failureDetail":{"reason":"WORKER_FAILED","message":"fixture worker failed"}
}`

const failureEvents = `[
  {"id":"failure-cursor-200","type":"FACTORY_SESSION_STARTED","schemaVersion":"agent-factory.event.v1","context":{"sequence":200,"sessionId":"dur-sess-terminal-failure","tick":1,"eventTime":"2026-07-16T00:00:00Z"},"payload":{}},
  {"id":"failure-cursor-201","type":"DISPATCH_RESPONSE","schemaVersion":"agent-factory.event.v1","context":{"sequence":201,"sessionId":"dur-sess-terminal-failure","tick":2,"eventTime":"2026-07-16T00:00:01Z"},"payload":{}},
  {"id":"failure-cursor-202","type":"SESSION_RESULT_UPDATED","schemaVersion":"agent-factory.event.v1","context":{"sequence":202,"sessionId":"dur-sess-terminal-failure","tick":3,"eventTime":"2026-07-16T00:00:02Z"},"payload":{}}
]`

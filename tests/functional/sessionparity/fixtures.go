package sessionparity

// CapturedObservations contains equivalent Factory Session observations from
// each customer interface. It is static scenario data for functional tests;
// callers can also supply their own captured observations to the normalizers.
type CapturedObservations struct {
	REST    []byte
	CLIJSON []byte
	MCP     []byte
}

// TerminalSuccessObservations returns deterministic captured observations for
// one completed Factory Session. Each call returns independent byte slices so
// a test cannot mutate the shared fixture data.
func TerminalSuccessObservations() CapturedObservations {
	return capturedObservations(terminalSuccessSession)
}

// TerminalFailureObservations returns deterministic captured observations for
// one inspectable terminal Factory Session failure. The fixture retains the
// affected dispatch and artifact alongside the terminal failure summary.
func TerminalFailureObservations() CapturedObservations {
	return capturedObservations(terminalFailureSession)
}

func capturedObservations(session string) CapturedObservations {
	return CapturedObservations{
		REST:    []byte(`{"session":` + session + `}`),
		CLIJSON: []byte(`{"factorySession":` + session + `}`),
		MCP:     []byte(`{"jsonrpc":"2.0","id":"fixture-request","result":{"factorySession":` + session + `}}`),
	}
}

const terminalSuccessSession = `{
  "identity":{"sessionId":"session-terminal-success"},
  "lifecycle":{"status":"SUCCEEDED","phase":"completed"},
  "hashes":{"sourceHash":"sha256:success-source","requestedPolicyHash":"sha256:success-requested-policy","effectivePolicyHash":"sha256:success-effective-policy"},
  "progress":{"totalDispatches":2,"completedDispatches":2,"failedDispatches":0,"inFlightDispatches":0},
  "dispatches":[
    {"sessionId":"session-terminal-success","id":"dispatch-success-1","order":1,"status":"SUCCEEDED","kind":"WORK"},
    {"sessionId":"session-terminal-success","id":"dispatch-success-2","order":2,"status":"SUCCEEDED","kind":"WORK"}
  ],
  "artifacts":[{"sessionId":"session-terminal-success","id":"artifact-success-result","order":1,"kind":"RESULT"}],
  "results":[{"sessionId":"session-terminal-success","id":"result-success-final","order":1,"status":"FINAL","value":"fixture success"}],
  "failures":[],
  "eventCursors":[
    {"sessionId":"session-terminal-success","cursor":"success-cursor-100","sequence":100,"eventType":"FACTORY_SESSION_STARTED"},
    {"sessionId":"session-terminal-success","cursor":"success-cursor-101","sequence":101,"eventType":"DISPATCH_COMPLETED"},
    {"sessionId":"session-terminal-success","cursor":"success-cursor-102","sequence":102,"eventType":"FACTORY_SESSION_COMPLETED"}
  ]
}`

const terminalFailureSession = `{
  "identity":{"sessionId":"session-terminal-failure"},
  "lifecycle":{"status":"FAILED","phase":"failed"},
  "hashes":{"sourceHash":"sha256:failure-source","requestedPolicyHash":"sha256:failure-requested-policy","effectivePolicyHash":"sha256:failure-effective-policy"},
  "progress":{"totalDispatches":2,"completedDispatches":1,"failedDispatches":1,"inFlightDispatches":0},
  "dispatches":[
    {"sessionId":"session-terminal-failure","id":"dispatch-failure-1","order":1,"status":"SUCCEEDED","kind":"WORK"},
    {"sessionId":"session-terminal-failure","id":"dispatch-failure-2","order":2,"status":"FAILED","kind":"WORK"}
  ],
  "artifacts":[{"sessionId":"session-terminal-failure","id":"artifact-failure-diagnostic","order":1,"kind":"DIAGNOSTIC"}],
  "results":[],
  "failures":[{"sessionId":"session-terminal-failure","id":"failure-terminal-1","order":1,"code":"WORKER_FAILED","message":"fixture worker failed","dispatchId":"dispatch-failure-2"}],
  "eventCursors":[
    {"sessionId":"session-terminal-failure","cursor":"failure-cursor-200","sequence":200,"eventType":"FACTORY_SESSION_STARTED"},
    {"sessionId":"session-terminal-failure","cursor":"failure-cursor-201","sequence":201,"eventType":"DISPATCH_FAILED"},
    {"sessionId":"session-terminal-failure","cursor":"failure-cursor-202","sequence":202,"eventType":"FACTORY_SESSION_FAILED"}
  ]
}`

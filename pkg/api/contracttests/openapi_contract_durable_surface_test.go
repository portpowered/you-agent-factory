package apicontract_test

import (
	"strings"
	"testing"
)

func assertDurableExecutionSurfaceSchemas(t *testing.T, schemas map[string]any, paths map[string]any) {
	t.Helper()

	asyncOperation := pathOperation(t, paths, "/factory-sessions/async", "post")
	if got, _ := asyncOperation["operationId"].(string); got != "startDurableFactorySessionAsync" {
		t.Fatalf("paths./factory-sessions/async.post.operationId = %q, want startDurableFactorySessionAsync", got)
	}
	assertRequestSchemaRef(t, asyncOperation, "#/components/schemas/FactorySessionExecutionRequest")
	assertResponseSchemaRef(t, asyncOperation, "202", "#/components/schemas/FactorySessionExecutionResponse")
	assertResponseRef(t, asyncOperation, "400", "#/components/responses/BadRequest")
	assertResponseRef(t, asyncOperation, "409", "#/components/responses/ExecutionRequestIdConflict")

	syncOperation := pathOperation(t, paths, "/factory-sessions/sync", "post")
	if got, _ := syncOperation["operationId"].(string); got != "startDurableFactorySessionSync" {
		t.Fatalf("paths./factory-sessions/sync.post.operationId = %q, want startDurableFactorySessionSync", got)
	}
	assertRequestSchemaRef(t, syncOperation, "#/components/schemas/FactorySessionExecutionRequest")
	assertResponseSchemaRef(t, syncOperation, "200", "#/components/schemas/FactorySessionSyncExecutionResponse")
	assertResponseRef(t, syncOperation, "400", "#/components/responses/BadRequest")
	assertResponseRef(t, syncOperation, "409", "#/components/responses/ExecutionRequestIdConflict")

	requestSchema := schemaObject(t, schemas, "FactorySessionExecutionRequest")
	assertRequiredFields(t, requestSchema, "requestId", "source")
	requestProperties := schemaProperties(t, requestSchema, "FactorySessionExecutionRequest")
	assertPropertyRef(t, requestProperties, "source", "#/components/schemas/FactorySessionExecutionSource")
	assertPropertyRef(t, requestProperties, "orchestrator", "#/components/schemas/FactoryOrchestrator")
	assertPropertyRef(t, requestProperties, "requestedPolicy", "#/components/schemas/FactorySessionRequestedPolicy")
	assertPropertyRef(t, requestProperties, "wait", "#/components/schemas/FactorySessionExecutionWaitOptions")
	assertSchemaPropertiesPresent(t, requestProperties, "FactorySessionExecutionRequest", "requestId", "source", "args", "orchestrator", "requestedPolicy", "wait")

	sourceSchema := schemaObject(t, schemas, "FactorySessionExecutionSource")
	assertRequiredFields(t, sourceSchema, "kind")
	sourceProperties := schemaProperties(t, sourceSchema, "FactorySessionExecutionSource")
	assertPropertyRef(t, sourceProperties, "kind", "#/components/schemas/FactorySessionExecutionSourceKind")
	assertSchemaPropertiesPresent(t, sourceProperties, "FactorySessionExecutionSource", "factoryId", "factoryInline", "workflowFile", "workflowName", "inlineWorkflow")
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionExecutionSourceKind"), "FactorySessionExecutionSourceKind", []string{
		"FACTORY_ID", "FACTORY_INLINE", "WORKFLOW_FILE", "WORKFLOW_NAME", "INLINE_WORKFLOW",
	})

	asyncResponseSchema := schemaObject(t, schemas, "FactorySessionExecutionResponse")
	assertRequiredFields(t, asyncResponseSchema, "sessionId", "status", "orchestratorKind", "resolvedSource")
	asyncResponseProperties := schemaProperties(t, asyncResponseSchema, "FactorySessionExecutionResponse")
	assertPropertyRef(t, asyncResponseProperties, "status", "#/components/schemas/FactorySessionDurableLifecycleStatus")
	assertPropertyRef(t, asyncResponseProperties, "orchestratorKind", "#/components/schemas/FactoryOrchestratorKind")
	assertPropertyRef(t, asyncResponseProperties, "resolvedSource", "#/components/schemas/FactorySessionResolvedSourceIdentity")
	assertPropertyRef(t, asyncResponseProperties, "links", "#/components/schemas/FactorySessionExecutionLinks")
	assertSchemaPropertiesPresent(t, asyncResponseProperties, "FactorySessionExecutionResponse", "sessionId", "status", "orchestratorKind", "dialect", "resolvedSource", "sourceHash", "requestedPolicy", "effectivePolicy", "effectivePolicyHash", "links")

	syncResponseSchema := schemaObject(t, schemas, "FactorySessionSyncExecutionResponse")
	assertRequiredFields(t, syncResponseSchema, "sessionId", "status", "orchestratorKind", "resolvedSource", "syncOutcome")
	syncResponseProperties := schemaProperties(t, syncResponseSchema, "FactorySessionSyncExecutionResponse")
	assertPropertyRef(t, syncResponseProperties, "syncOutcome", "#/components/schemas/FactorySessionSyncExecutionOutcome")
	assertPropertyRef(t, syncResponseProperties, "result", "#/components/schemas/FactorySessionResult")
	assertSchemaPropertiesPresent(t, syncResponseProperties, "FactorySessionSyncExecutionResponse", "requestedPolicy", "effectivePolicy", "effectivePolicyHash", "syncOutcome", "result", "timedOut", "sessionCanceledByTimeout")
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionSyncExecutionOutcome"), "FactorySessionSyncExecutionOutcome", []string{"COMPLETED", "TIMED_OUT", "STILL_RUNNING"})

	waitOptions := schemaObject(t, schemas, "FactorySessionExecutionWaitOptions")
	waitProperties := schemaProperties(t, waitOptions, "FactorySessionExecutionWaitOptions")
	assertSchemaPropertiesPresent(t, waitProperties, "FactorySessionExecutionWaitOptions", "timeoutMillis", "cancelOnTimeout")

	openFactorySession := pathOperation(t, paths, "/factory-sessions", "post")
	openDescription, _ := openFactorySession["description"].(string)
	if !strings.Contains(strings.ToLower(openDescription), "not the primary durable") {
		t.Fatalf("paths./factory-sessions.post.description must document live-session compatibility, got %q", openDescription)
	}
	invokeOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/invocations", "post")
	invokeDescription, _ := invokeOperation["description"].(string)
	if !strings.Contains(strings.ToLower(invokeDescription), "not the primary durable") {
		t.Fatalf("paths./factory-sessions/{session_id}/invocations.post.description must document live-session compatibility, got %q", invokeDescription)
	}
}

func assertDurableSourceResolutionAndIdempotencySurfaceSchemas(t *testing.T, schemas map[string]any, paths map[string]any) {
	t.Helper()

	asyncOperation := pathOperation(t, paths, "/factory-sessions/async", "post")
	asyncDescription, _ := asyncOperation["description"].(string)
	for _, fragment := range []string{
		".claude/workflows",
		"~/.you-agent-factory/workflows",
		"package-relative workflow directories",
		"built-in/global JavaScript factories",
		"explicit factory lookup",
		"EXECUTION_REQUEST_ID_CONFLICT",
	} {
		if !strings.Contains(asyncDescription, fragment) {
			t.Fatalf("paths./factory-sessions/async.post.description must document %q, got %q", fragment, asyncDescription)
		}
	}

	syncOperation := pathOperation(t, paths, "/factory-sessions/sync", "post")
	syncDescription, _ := syncOperation["description"].(string)
	if !strings.Contains(strings.ToLower(syncDescription), "idempotency") {
		t.Fatalf("paths./factory-sessions/sync.post.description must document requestId idempotency, got %q", syncDescription)
	}

	sourceSchema := schemaObject(t, schemas, "FactorySessionExecutionSource")
	sourceDescription, _ := sourceSchema["description"].(string)
	if !strings.Contains(sourceDescription, "FactorySessionWorkflowSourceResolutionOrder") {
		t.Fatalf("components.schemas.FactorySessionExecutionSource.description must reference FactorySessionWorkflowSourceResolutionOrder")
	}

	resolvedSourceSchema := schemaObject(t, schemas, "FactorySessionResolvedSourceIdentity")
	resolvedSourceProperties := schemaProperties(t, resolvedSourceSchema, "FactorySessionResolvedSourceIdentity")
	assertPropertyRef(t, resolvedSourceProperties, "kind", "#/components/schemas/FactorySessionExecutionSourceKind")
	assertArrayItemRef(t, resolvedSourceProperties, "resolutionOrder", "#/components/schemas/FactorySessionWorkflowSourceResolutionOrder")
	assertSchemaPropertiesPresent(t, resolvedSourceProperties, "FactorySessionResolvedSourceIdentity",
		"kind", "sourceRef", "sourceHash", "dialect", "metadata", "resolutionOrder")

	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionWorkflowSourceResolutionOrder"), "FactorySessionWorkflowSourceResolutionOrder", []string{
		"PROJECT_CLAUDE_WORKFLOWS",
		"USER_YOU_AGENT_FACTORY_WORKFLOWS",
		"PACKAGE_RELATIVE_WORKFLOW_DIRECTORIES",
		"BUILTIN_GLOBAL_JAVASCRIPT_FACTORIES",
		"EXPLICIT_FACTORY_LOOKUP",
	})

	requestedPolicySchema := schemaObject(t, schemas, "FactorySessionRequestedPolicy")
	requestedPolicyProperties := schemaProperties(t, requestedPolicySchema, "FactorySessionRequestedPolicy")
	assertSchemaPropertiesPresent(t, requestedPolicyProperties, "FactorySessionRequestedPolicy", "policyHash")

	effectivePolicySchema := schemaObject(t, schemas, "FactorySessionEffectivePolicy")
	effectivePolicyProperties := schemaProperties(t, effectivePolicySchema, "FactorySessionEffectivePolicy")
	assertSchemaPropertiesPresent(t, effectivePolicyProperties, "FactorySessionEffectivePolicy", "policyHash")

	requestSchema := schemaObject(t, schemas, "FactorySessionExecutionRequest")
	requestDescription, _ := requestSchema["description"].(string)
	if !strings.Contains(strings.ToLower(requestDescription), "idempotency") {
		t.Fatalf("components.schemas.FactorySessionExecutionRequest.description must document idempotency normalization")
	}
	requestProperties := schemaProperties(t, requestSchema, "FactorySessionExecutionRequest")
	assertPropertyRef(t, requestProperties, "requestedPolicy", "#/components/schemas/FactorySessionRequestedPolicy")

	asyncResponseProperties := schemaProperties(t, schemaObject(t, schemas, "FactorySessionExecutionResponse"), "FactorySessionExecutionResponse")
	assertPropertyRef(t, asyncResponseProperties, "requestedPolicy", "#/components/schemas/FactorySessionRequestedPolicy")
	assertPropertyRef(t, asyncResponseProperties, "effectivePolicy", "#/components/schemas/FactorySessionEffectivePolicy")

	errorSchema := schemaObject(t, schemas, "ErrorResponse")
	codeProperty, ok := schemaProperties(t, errorSchema, "ErrorResponse")["code"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.ErrorResponse.properties.code is missing")
	}
	codeEnum, ok := codeProperty["enum"].([]any)
	if !ok || !containsString(codeEnum, "EXECUTION_REQUEST_ID_CONFLICT") {
		t.Fatalf("components.schemas.ErrorResponse.properties.code.enum is missing EXECUTION_REQUEST_ID_CONFLICT")
	}
}

func assertDurableSessionReadSurfaceSchemas(t *testing.T, schemas map[string]any, paths map[string]any) {
	t.Helper()

	listOperation := pathOperation(t, paths, "/factory-sessions", "get")
	if got, _ := listOperation["operationId"].(string); got != "listFactorySessions" {
		t.Fatalf("paths./factory-sessions.get.operationId = %q, want listFactorySessions", got)
	}
	assertResponseSchemaRef(t, listOperation, "200", "#/components/schemas/ListFactorySessionsResponse")
	assertResponseRef(t, listOperation, "400", "#/components/responses/BadRequest")
	parameters, ok := listOperation["parameters"].([]any)
	if !ok {
		t.Fatalf("paths./factory-sessions.get.parameters is missing")
	}
	assertParameterRef(t, parameters, "#/components/parameters/FactorySessionListScope")

	listResponseSchema := schemaObject(t, schemas, "ListFactorySessionsResponse")
	assertRequiredFields(t, listResponseSchema, "sessions")
	listResponseProperties := schemaProperties(t, listResponseSchema, "ListFactorySessionsResponse")
	assertPropertyRef(t, listResponseProperties, "scope", "#/components/schemas/FactorySessionListScope")
	assertArrayItemRef(t, listResponseProperties, "sessions", "#/components/schemas/FactorySessionSummary")
	assertArrayItemRef(t, listResponseProperties, "durableSessions", "#/components/schemas/FactorySessionDurableSummary")
	assertSchemaPropertiesPresent(t, listResponseProperties, "ListFactorySessionsResponse", "scope", "sessions", "durableSessions")
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionListScope"), "FactorySessionListScope", []string{"LIVE", "PERSISTED", "ALL"})

	getOperation := pathOperation(t, paths, "/factory-sessions/{session_id}", "get")
	if got, _ := getOperation["operationId"].(string); got != "getFactorySession" {
		t.Fatalf("paths./factory-sessions/{session_id}.get.operationId = %q, want getFactorySession", got)
	}
	assertResponseSchemaRef(t, getOperation, "200", "#/components/schemas/FactorySessionGetResponse")
	assertResponseRef(t, getOperation, "404", "#/components/responses/NotFound")

	getResponseSchema := schemaObject(t, schemas, "FactorySessionGetResponse")
	assertSchemaOneOfRefs(t, getResponseSchema, "FactorySessionGetResponse", []string{
		"#/components/schemas/FactorySession",
		"#/components/schemas/FactorySessionDurableReadModel",
	})

	durableReadModel := schemaObject(t, schemas, "FactorySessionDurableReadModel")
	assertRequiredFields(t, durableReadModel, "sessionId", "status", "orchestratorKind", "resolvedSource")
	durableReadModelProperties := schemaProperties(t, durableReadModel, "FactorySessionDurableReadModel")
	assertPropertyRef(t, durableReadModelProperties, "status", "#/components/schemas/FactorySessionDurableLifecycleStatus")
	assertPropertyRef(t, durableReadModelProperties, "orchestratorKind", "#/components/schemas/FactoryOrchestratorKind")
	assertPropertyRef(t, durableReadModelProperties, "resolvedSource", "#/components/schemas/FactorySessionResolvedSourceIdentity")
	assertArrayItemRef(t, durableReadModelProperties, "phaseSummaries", "#/components/schemas/FactorySessionDurablePhaseSummary")
	assertPropertyRef(t, durableReadModelProperties, "progress", "#/components/schemas/FactorySessionDurableProgressCounts")
	assertPropertyRef(t, durableReadModelProperties, "budgets", "#/components/schemas/FactorySessionBudgets")
	assertPropertyRef(t, durableReadModelProperties, "usage", "#/components/schemas/FactorySessionUsage")
	assertArrayItemRef(t, durableReadModelProperties, "artifactRefs", "#/components/schemas/FactoryArtifactRef")
	assertPropertyRef(t, durableReadModelProperties, "resultSummary", "#/components/schemas/FactorySessionDurableResultSummary")
	assertPropertyRef(t, durableReadModelProperties, "failure", "#/components/schemas/FactorySessionDurableFailureDetail")
	assertPropertyRef(t, durableReadModelProperties, "lifecycle", "#/components/schemas/FactorySessionDurableLifecycleTimestamps")
	assertPropertyRef(t, durableReadModelProperties, "links", "#/components/schemas/FactorySessionExecutionLinks")
	assertPropertyRef(t, durableReadModelProperties, "requestedPolicy", "#/components/schemas/FactorySessionRequestedPolicy")
	assertPropertyRef(t, durableReadModelProperties, "effectivePolicy", "#/components/schemas/FactorySessionEffectivePolicy")
	assertSchemaPropertiesPresent(t, durableReadModelProperties, "FactorySessionDurableReadModel",
		"sessionId", "status", "orchestratorKind", "dialect", "resolvedSource", "sourceHash",
		"requestedPolicy", "effectivePolicy", "effectivePolicyHash", "phase", "phaseSummaries", "progress", "budgets", "usage",
		"artifactRefs", "resultSummary", "failure", "lifecycle", "staleLease", "links")

	durableSummary := schemaObject(t, schemas, "FactorySessionDurableSummary")
	assertRequiredFields(t, durableSummary, "sessionId", "status", "orchestratorKind", "resolvedSource")
	durableSummaryProperties := schemaProperties(t, durableSummary, "FactorySessionDurableSummary")
	assertPropertyRef(t, durableSummaryProperties, "status", "#/components/schemas/FactorySessionDurableLifecycleStatus")
	assertPropertyRef(t, durableSummaryProperties, "resolvedSource", "#/components/schemas/FactorySessionResolvedSourceIdentity")
	assertPropertyRef(t, durableSummaryProperties, "requestedPolicy", "#/components/schemas/FactorySessionRequestedPolicy")
	assertPropertyRef(t, durableSummaryProperties, "effectivePolicy", "#/components/schemas/FactorySessionEffectivePolicy")
	assertSchemaPropertiesPresent(t, durableSummaryProperties, "FactorySessionDurableSummary",
		"sessionId", "status", "orchestratorKind", "dialect", "resolvedSource", "sourceHash",
		"requestedPolicy", "effectivePolicy", "effectivePolicyHash", "staleLease", "lifecycle", "links")

	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionDurableLifecycleStatus"), "FactorySessionDurableLifecycleStatus", []string{
		"QUEUED", "AWAITING_APPROVAL", "RUNNING", "PAUSED", "RESUMING", "SUCCEEDED", "FAILED",
		"CANCELING", "CANCELED", "TIMED_OUT", "INTERRUPTED", "TERMINATED",
	})
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionResultStatus"), "FactorySessionResultStatus", []string{
		"NOT_READY", "PARTIAL", "FINAL", "FAILED_WITH_PARTIAL", "UNAVAILABLE",
	})

	resultSummary := schemaObject(t, schemas, "FactorySessionDurableResultSummary")
	assertRequiredFields(t, resultSummary, "resultStatus")
	assertPropertyRef(t, schemaProperties(t, resultSummary, "FactorySessionDurableResultSummary"), "resultStatus", "#/components/schemas/FactorySessionResultStatus")
}

func assertDurableSessionResultSurfaceSchemas(t *testing.T, schemas map[string]any, paths map[string]any) {
	t.Helper()

	liveResultOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/result", "get")
	assertResponseSchemaRef(t, liveResultOperation, "200", "#/components/schemas/FactorySessionLiveResult")

	resultsOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/results", "get")
	if got, _ := resultsOperation["operationId"].(string); got != "getFactorySessionResults" {
		t.Fatalf("paths./factory-sessions/{session_id}/results.get.operationId = %q, want getFactorySessionResults", got)
	}
	assertResponseSchemaRef(t, resultsOperation, "200", "#/components/schemas/FactorySessionResult")
	assertResponseRef(t, resultsOperation, "404", "#/components/responses/NotFound")
	parameters, ok := resultsOperation["parameters"].([]any)
	if !ok {
		t.Fatalf("paths./factory-sessions/{session_id}/results.get.parameters is missing")
	}
	assertParameterRef(t, parameters, "#/components/parameters/FactorySessionResultMode")
	assertParameterRef(t, parameters, "#/components/parameters/FactorySessionResultIncludeArtifacts")

	resultSchema := schemaObject(t, schemas, "FactorySessionResult")
	assertRequiredFields(t, resultSchema, "sessionId", "resultStatus")
	resultProperties := schemaProperties(t, resultSchema, "FactorySessionResult")
	assertPropertyRef(t, resultProperties, "resultStatus", "#/components/schemas/FactorySessionResultStatus")
	assertPropertyRef(t, resultProperties, "sessionStatus", "#/components/schemas/FactorySessionDurableLifecycleStatus")
	assertPropertyRef(t, resultProperties, "mode", "#/components/schemas/FactorySessionResultMode")
	assertPropertyRef(t, resultProperties, "primaryResult", "#/components/schemas/WorkContent")
	assertArrayItemRef(t, resultProperties, "artifactRefs", "#/components/schemas/FactoryArtifactRef")
	assertPropertyRef(t, resultProperties, "failure", "#/components/schemas/FactorySessionDurableFailureDetail")
	assertPropertyRef(t, resultProperties, "availability", "#/components/schemas/FactorySessionResultAvailabilityDetail")
	assertSchemaPropertiesPresent(t, resultProperties, "FactorySessionResult",
		"sessionId", "resultStatus", "sessionStatus", "mode", "includeArtifacts",
		"primaryResult", "artifactIds", "artifactRefs", "failure", "availability")
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionResultMode"), "FactorySessionResultMode", []string{"FINAL", "PARTIAL"})

	liveResultSchema := schemaObject(t, schemas, "FactorySessionLiveResult")
	assertRequiredFields(t, liveResultSchema, "sessionId", "status")
	liveResultProperties := schemaProperties(t, liveResultSchema, "FactorySessionLiveResult")
	assertPropertyRef(t, liveResultProperties, "status", "#/components/schemas/FactorySessionStatus")
	assertPropertyRef(t, liveResultProperties, "resultArtifactRef", "#/components/schemas/FactoryArtifactRef")
}

func assertDurableSessionDispatchArtifactSurfaceSchemas(t *testing.T, schemas map[string]any, paths map[string]any) {
	t.Helper()

	dispatchesOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/dispatches", "get")
	if got, _ := dispatchesOperation["operationId"].(string); got != "listFactorySessionDispatches" {
		t.Fatalf("paths./factory-sessions/{session_id}/dispatches.get.operationId = %q, want listFactorySessionDispatches", got)
	}
	assertResponseSchemaRef(t, dispatchesOperation, "200", "#/components/schemas/ListFactorySessionDispatchesResponse")
	assertResponseRef(t, dispatchesOperation, "404", "#/components/responses/NotFound")

	dispatchOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/dispatches/{dispatch_id}", "get")
	if got, _ := dispatchOperation["operationId"].(string); got != "getFactorySessionDispatch" {
		t.Fatalf("paths./factory-sessions/{session_id}/dispatches/{dispatch_id}.get.operationId = %q, want getFactorySessionDispatch", got)
	}
	assertResponseSchemaRef(t, dispatchOperation, "200", "#/components/schemas/FactoryDispatch")
	assertResponseRef(t, dispatchOperation, "404", "#/components/responses/NotFound")
	dispatchParameters, ok := dispatchOperation["parameters"].([]any)
	if !ok {
		t.Fatalf("paths./factory-sessions/{session_id}/dispatches/{dispatch_id}.get.parameters is missing")
	}
	assertParameterRef(t, dispatchParameters, "#/components/parameters/DispatchID")

	artifactsOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/artifacts", "get")
	if got, _ := artifactsOperation["operationId"].(string); got != "listFactorySessionArtifacts" {
		t.Fatalf("paths./factory-sessions/{session_id}/artifacts.get.operationId = %q, want listFactorySessionArtifacts", got)
	}
	assertResponseSchemaRef(t, artifactsOperation, "200", "#/components/schemas/ListFactorySessionArtifactsResponse")
	assertResponseRef(t, artifactsOperation, "404", "#/components/responses/NotFound")

	artifactOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/artifacts/{artifact_id}", "get")
	if got, _ := artifactOperation["operationId"].(string); got != "getFactorySessionArtifact" {
		t.Fatalf("paths./factory-sessions/{session_id}/artifacts/{artifact_id}.get.operationId = %q, want getFactorySessionArtifact", got)
	}
	assertResponseSchemaRef(t, artifactOperation, "200", "#/components/schemas/FactorySessionArtifactDetail")
	assertResponseRef(t, artifactOperation, "404", "#/components/responses/NotFound")
	artifactParameters, ok := artifactOperation["parameters"].([]any)
	if !ok {
		t.Fatalf("paths./factory-sessions/{session_id}/artifacts/{artifact_id}.get.parameters is missing")
	}
	assertParameterRef(t, artifactParameters, "#/components/parameters/ArtifactID")

	listDispatchesSchema := schemaObject(t, schemas, "ListFactorySessionDispatchesResponse")
	assertRequiredFields(t, listDispatchesSchema, "sessionId", "dispatches")
	listDispatchesProperties := schemaProperties(t, listDispatchesSchema, "ListFactorySessionDispatchesResponse")
	assertArrayItemRef(t, listDispatchesProperties, "dispatches", "#/components/schemas/FactorySessionDispatchSummary")

	dispatchSummarySchema := schemaObject(t, schemas, "FactorySessionDispatchSummary")
	assertRequiredFields(t, dispatchSummarySchema, "id", "status", "dispatchKind")
	dispatchSummaryProperties := schemaProperties(t, dispatchSummarySchema, "FactorySessionDispatchSummary")
	assertPropertyRef(t, dispatchSummaryProperties, "status", "#/components/schemas/FactoryDispatchStatus")
	assertPropertyRef(t, dispatchSummaryProperties, "dispatchKind", "#/components/schemas/FactoryDispatchKind")
	assertPropertyRef(t, dispatchSummaryProperties, "usage", "#/components/schemas/FactoryDispatchUsage")
	assertPropertyRef(t, dispatchSummaryProperties, "failureDetail", "#/components/schemas/FactoryDispatchFailureDetail")
	assertArrayItemRef(t, dispatchSummaryProperties, "providerSessionRefs", "#/components/schemas/LoadableProviderSessionRef")
	assertArrayItemRef(t, dispatchSummaryProperties, "warnings", "#/components/schemas/FactoryDispatchWarning")
	assertSchemaPropertiesPresent(t, dispatchSummaryProperties, "FactorySessionDispatchSummary",
		"id", "status", "dispatchKind", "phase", "label", "attempt", "runnerId", "model", "provider",
		"providerSessionRefs", "usage", "warnings", "outputArtifactIds", "failureDetail")

	dispatchDetailSchema := schemaObject(t, schemas, "FactoryDispatch")
	dispatchDetailProperties := schemaProperties(t, dispatchDetailSchema, "FactoryDispatch")
	assertPropertyRef(t, dispatchDetailProperties, "petri", "#/components/schemas/FactoryDispatchPetriProjection")
	assertPropertyRef(t, dispatchDetailProperties, "javascript", "#/components/schemas/FactoryDispatchJavaScriptProjection")
	assertArrayItemRef(t, dispatchDetailProperties, "providerSessionRefs", "#/components/schemas/LoadableProviderSessionRef")
	assertSchemaPropertiesPresent(t, dispatchDetailProperties, "FactoryDispatch", "attempt", "providerSessionRefs")

	listArtifactsSchema := schemaObject(t, schemas, "ListFactorySessionArtifactsResponse")
	assertRequiredFields(t, listArtifactsSchema, "sessionId", "artifacts")
	listArtifactsProperties := schemaProperties(t, listArtifactsSchema, "ListFactorySessionArtifactsResponse")
	assertArrayItemRef(t, listArtifactsProperties, "artifacts", "#/components/schemas/FactorySessionArtifactSummary")

	artifactSummarySchema := schemaObject(t, schemas, "FactorySessionArtifactSummary")
	assertRequiredFields(t, artifactSummarySchema, "id", "kind", "visibility")
	artifactSummaryProperties := schemaProperties(t, artifactSummarySchema, "FactorySessionArtifactSummary")
	assertPropertyRef(t, artifactSummaryProperties, "kind", "#/components/schemas/FactoryArtifactKind")
	assertPropertyRef(t, artifactSummaryProperties, "visibility", "#/components/schemas/FactoryArtifactVisibility")
	assertPropertyRef(t, artifactSummaryProperties, "auditMode", "#/components/schemas/FactoryArtifactAuditMode")
	assertPropertyRef(t, artifactSummaryProperties, "redactionCounts", "#/components/schemas/FactoryArtifactRedactionCounts")
	assertPropertyRef(t, artifactSummaryProperties, "retrievalRef", "#/components/schemas/FactorySessionArtifactRetrievalRef")
	assertSchemaPropertiesPresent(t, artifactSummaryProperties, "FactorySessionArtifactSummary",
		"id", "kind", "visibility", "contentHash", "sizeBytes", "createdAt", "dispatchId",
		"auditMode", "redactionCounts", "retrievalRef")

	artifactDetailSchema := schemaObject(t, schemas, "FactorySessionArtifactDetail")
	assertRequiredFields(t, artifactDetailSchema, "sessionId", "id", "kind", "visibility")
	artifactDetailProperties := schemaProperties(t, artifactDetailSchema, "FactorySessionArtifactDetail")
	assertPropertyRef(t, artifactDetailProperties, "content", "#/components/schemas/WorkContent")
	assertPropertyRef(t, artifactDetailProperties, "contentRef", "#/components/schemas/FactorySessionArtifactRetrievalRef")
	assertSchemaPropertiesPresent(t, artifactDetailProperties, "FactorySessionArtifactDetail",
		"sessionId", "id", "kind", "visibility", "contentHash", "sizeBytes", "createdAt", "dispatchId",
		"auditMode", "redactionCounts", "captureMetadata", "content", "contentRef")
}

func assertDurableSessionLifecycleControlSurfaceSchemas(t *testing.T, schemas map[string]any, paths map[string]any) {
	t.Helper()

	lifecycleRoutes := []struct {
		path          string
		operationID   string
		requestSchema string
		requiredBody  bool
	}{
		{"/factory-sessions/{session_id}/approve", "approveFactorySession", "#/components/schemas/FactorySessionApproveRequest", false},
		{"/factory-sessions/{session_id}/pause", "pauseFactorySession", "#/components/schemas/FactorySessionLifecycleControlRequest", false},
		{"/factory-sessions/{session_id}/resume", "resumeFactorySession", "#/components/schemas/FactorySessionLifecycleControlRequest", false},
		{"/factory-sessions/{session_id}/cancel", "cancelFactorySession", "#/components/schemas/FactorySessionLifecycleControlRequest", false},
		{"/factory-sessions/{session_id}/terminate", "terminateFactorySession", "#/components/schemas/FactorySessionLifecycleControlRequest", false},
		{"/factory-sessions/{session_id}/retry-dispatch", "retryFactorySessionDispatch", "#/components/schemas/FactorySessionRetryDispatchRequest", true},
	}
	for _, route := range lifecycleRoutes {
		operation := pathOperation(t, paths, route.path, "post")
		if got, _ := operation["operationId"].(string); got != route.operationID {
			t.Fatalf("paths.%s.post.operationId = %q, want %s", route.path, got, route.operationID)
		}
		if route.requiredBody {
			assertRequestSchemaRef(t, operation, route.requestSchema)
		}
		assertResponseSchemaRef(t, operation, "200", "#/components/schemas/FactorySessionLifecycleControlResponse")
		assertResponseSchemaRef(t, operation, "202", "#/components/schemas/FactorySessionLifecycleControlResponse")
		assertResponseRef(t, operation, "404", "#/components/responses/NotFound")
		assertResponseRef(t, operation, "409", "#/components/responses/FactorySessionLifecycleControlConflict")
	}

	retryOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/retry-dispatch", "post")
	assertRequestSchemaRef(t, retryOperation, "#/components/schemas/FactorySessionRetryDispatchRequest")
	retryRequestSchema := schemaObject(t, schemas, "FactorySessionRetryDispatchRequest")
	assertRequiredFields(t, retryRequestSchema, "dispatchId")

	controlResponseSchema := schemaObject(t, schemas, "FactorySessionLifecycleControlResponse")
	assertRequiredFields(t, controlResponseSchema, "sessionId", "operation", "outcome", "status")
	controlResponseProperties := schemaProperties(t, controlResponseSchema, "FactorySessionLifecycleControlResponse")
	assertPropertyRef(t, controlResponseProperties, "operation", "#/components/schemas/FactorySessionLifecycleControlKind")
	assertPropertyRef(t, controlResponseProperties, "outcome", "#/components/schemas/FactorySessionLifecycleControlOutcome")
	assertPropertyRef(t, controlResponseProperties, "status", "#/components/schemas/FactorySessionDurableLifecycleStatus")
	assertPropertyRef(t, controlResponseProperties, "session", "#/components/schemas/FactorySessionDurableReadModel")
	assertPropertyRef(t, controlResponseProperties, "links", "#/components/schemas/FactorySessionLifecycleControlLinks")
	assertSchemaPropertiesPresent(t, controlResponseProperties, "FactorySessionLifecycleControlResponse",
		"sessionId", "operation", "outcome", "status", "session", "effectivePolicyHash",
		"approvalPreviewId", "dispatchId", "retryDispatchId", "detail", "links")

	approveRequestSchema := schemaObject(t, schemas, "FactorySessionApproveRequest")
	approveRequestProperties := schemaProperties(t, approveRequestSchema, "FactorySessionApproveRequest")
	assertSchemaPropertiesPresent(t, approveRequestProperties, "FactorySessionApproveRequest",
		"requestId", "reason", "approvalPreviewId", "approvedPolicy")

	controlLinksSchema := schemaObject(t, schemas, "FactorySessionLifecycleControlLinks")
	controlLinksProperties := schemaProperties(t, controlLinksSchema, "FactorySessionLifecycleControlLinks")
	assertSchemaPropertiesPresent(t, controlLinksProperties, "FactorySessionLifecycleControlLinks",
		"session", "results", "dispatches", "artifacts", "events", "status")

	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionLifecycleControlKind"), "FactorySessionLifecycleControlKind", []string{
		"APPROVE", "PAUSE", "RESUME", "CANCEL", "TERMINATE", "RETRY_DISPATCH",
	})
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionLifecycleControlOutcome"), "FactorySessionLifecycleControlOutcome", []string{
		"ACCEPTED", "NO_OP", "INVALID_STATE", "TERMINAL_SESSION", "CONFLICT",
	})
}

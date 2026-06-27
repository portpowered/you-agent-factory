import type { DashboardSnapshot } from "../api/dashboard";
import { FactoryOrchestratorKind } from "../api/generated/openapi";
import type {
  FactorySession,
  FactorySessionSummary,
} from "../api/factory-sessions/api";

function jsonResponse(
  body: unknown,
  status = 200,
  statusText?: string,
): Response {
  return new Response(JSON.stringify(body), {
    headers: {
      "Content-Type": "application/json",
    },
    status,
    statusText,
  });
}

export function buildFactorySessionResponse(
  summary: FactorySessionSummary,
  snapshot: DashboardSnapshot,
): FactorySession {
  const lifecycleTimestamp = "2026-06-26T00:00:00Z";

  return {
    factoryDir: summary.factoryDir,
    folderPath: summary.folderPath,
    id: summary.id,
    isDefault: summary.isDefault,
    project: summary.project,
    runtime: {
      lifecycle: {
        startedAt: lifecycleTimestamp,
        updatedAt: lifecycleTimestamp,
      },
      orchestratorKind: FactoryOrchestratorKind.PETRI,
      progress: {
        categories: {
          failed: 0,
          initial: 0,
          processing: 0,
          terminal: 0,
        },
        factoryState: snapshot.factory_state,
        inFlightCount: 0,
        totalTokens: 0,
      },
      status: "IDLE",
      streamIdentity: {
        backendScopeID: `${summary.folderPath}::test-backend`,
        factorySessionID: summary.id,
        streamGenerationID: lifecycleTimestamp,
      },
      usage: { resources: [] },
    },
    target: summary.target,
  };
}

export function handleFactorySessionPreflightRequest({
  availableFactorySessions,
  method,
  path,
  snapshot,
}: {
  availableFactorySessions: readonly FactorySessionSummary[];
  method: string;
  path: string;
  snapshot: DashboardSnapshot;
}): Response | undefined {
  if (path === "/factory-sessions") {
    return jsonResponse({
      sessions: availableFactorySessions,
    });
  }

  if (method !== "GET") {
    return undefined;
  }

  const syncPreflightMatch = path.match(
    /^\/factory-sessions\/([^/]+)\/sync-preflight(?:\?(.*))?$/,
  );
  if (syncPreflightMatch) {
    const requestedSessionID = decodeURIComponent(syncPreflightMatch[1] ?? "");
    const sessionSummary = availableFactorySessions.find(
      (session) => session.id === requestedSessionID,
    );

    if (!sessionSummary) {
      return jsonResponse(
        {
          code: "FACTORY_SESSION_NOT_FOUND",
          message: `Factory session ${requestedSessionID} was not found.`,
        },
        404,
        "Not Found",
      );
    }

    const searchParams = new URLSearchParams(syncPreflightMatch[2] ?? "");
    const afterSequence = searchParams.get("after_sequence");

    return jsonResponse({
      backendScopeId: `${sessionSummary.folderPath}::test-backend`,
      checkpointReusable: true,
      factorySessionId: sessionSummary.id,
      logicalSessionKeyId: `${sessionSummary.folderPath}::${sessionSummary.id}`,
      reasonCode: "ok",
      reconnectCursor: {
        afterEventId: searchParams.get("after_event_id") ?? undefined,
        afterSequence:
          afterSequence == null ? undefined : Number.parseInt(afterSequence, 10),
        provided: searchParams.has("after_event_id") || afterSequence != null,
        validForStreamGeneration: true,
      },
      requestedSessionId: requestedSessionID,
      streamGenerationId: buildFactorySessionResponse(sessionSummary, snapshot)
        .runtime.streamIdentity.streamGenerationID,
    });
  }

  if (!/^\/factory-sessions\/[^/]+$/.test(path)) {
    return undefined;
  }

  const requestedSessionID = decodeURIComponent(
    path.slice("/factory-sessions/".length),
  );
  const sessionSummary = availableFactorySessions.find(
    (session) => session.id === requestedSessionID,
  );

  if (!sessionSummary) {
    return jsonResponse(
      {
        code: "FACTORY_SESSION_NOT_FOUND",
        message: `Factory session ${requestedSessionID} was not found.`,
      },
      404,
      "Not Found",
    );
  }

  return jsonResponse(buildFactorySessionResponse(sessionSummary, snapshot));
}

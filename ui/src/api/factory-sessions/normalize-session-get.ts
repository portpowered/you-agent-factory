import type { components } from "../generated/openapi";
import {
  FactoryOrchestratorKind,
  FactorySessionDurableLifecycleStatus,
  FactorySessionJavaScriptScriptStatus,
  FactorySessionStatus,
} from "../generated/openapi";
import { withoutEmbeddedSessionDispatches } from "./dispatch-free-session";
import { resultSurfacesFromDurableReadModel } from "./normalize-durable-inspection";

export type FactorySession = components["schemas"]["FactorySession"];
export type FactorySessionDurableReadModel =
  components["schemas"]["FactorySessionDurableReadModel"];
export type FactorySessionLiveResult =
  components["schemas"]["FactorySessionLiveResult"];
export type FactorySessionPartialResult =
  components["schemas"]["FactorySessionPartialResult"];

export interface NormalizedFactorySessionGet {
  durableReadModel?: FactorySessionDurableReadModel;
  durableLifecycleStatus?: components["schemas"]["FactorySessionDurableLifecycleStatus"];
  durableProgress?: components["schemas"]["FactorySessionDurableProgressCounts"];
  partialResult?: FactorySessionPartialResult;
  result?: FactorySessionLiveResult;
  resultSummary?: FactorySessionDurableReadModel["resultSummary"];
  session: FactorySession;
}

export function normalizeFactorySessionGetResponse(
  responseBody: unknown,
): NormalizedFactorySessionGet {
  if (isFactorySession(responseBody)) {
    return { session: withoutEmbeddedSessionDispatches(responseBody) };
  }

  if (isFactorySessionDurableReadModel(responseBody)) {
    const resultSurfaces = resultSurfacesFromDurableReadModel(responseBody);
    return {
      durableReadModel: responseBody,
      durableLifecycleStatus: responseBody.status,
      durableProgress: responseBody.progress,
      partialResult: resultSurfaces.partialResult,
      result: resultSurfaces.result,
      resultSummary: responseBody.resultSummary,
      session: factorySessionFromDurableReadModel(responseBody),
    };
  }

  throw new Error(
    "Factory session GET response is not a recognized session shape.",
  );
}

function factorySessionFromDurableReadModel(
  durable: FactorySessionDurableReadModel,
): FactorySession {
  const lifecycle = durableLifecycleFromDurable(durable);
  const progress = durable.progress;

  return {
    factoryDir: "",
    folderPath: "",
    id: durable.sessionId,
    isDefault: false,
    project: durable.resolvedSource.sourceRef ?? durable.sessionId,
    runtime: {
      artifacts: artifactRefsToArtifacts(durable.artifactRefs),
      budgets: durable.budgets,
      dialect: durable.dialect,
      javascript:
        durable.orchestratorKind === FactoryOrchestratorKind.JAVASCRIPT
          ? {
              childDispatchCounts:
                childDispatchCountsFromDurableProgress(progress),
              phase: durable.phase,
              phases: phaseNamesFromDurable(durable),
              scriptStatus: scriptStatusFromDurableLifecycle(durable.status),
            }
          : undefined,
      lifecycle,
      orchestratorKind: durable.orchestratorKind,
      policyHash: durable.effectivePolicyHash,
      progress: {
        categories: {
          failed: 0,
          initial: 0,
          processing: 0,
          terminal: 0,
        },
        factoryState: durable.status,
        inFlightCount: progress?.inFlightDispatches ?? 0,
        totalTokens: 0,
      },
      sourceHash: durable.sourceHash,
      sourceRef: durable.resolvedSource.sourceRef,
      status: runtimeStatusFromDurableLifecycle(durable.status),
      usage: durable.usage ?? { resources: [] },
    },
    target: {
      kind: "named",
      name: durable.sessionId,
    },
  };
}

function durableLifecycleFromDurable(
  durable: FactorySessionDurableReadModel,
): components["schemas"]["FactorySessionLifecycle"] {
  const timestamps = durable.lifecycle;
  const fallbackTimestamp = "1970-01-01T00:00:00Z";

  return {
    finishedAt: timestamps?.finishedAt,
    startedAt:
      timestamps?.startedAt ?? timestamps?.queuedAt ?? fallbackTimestamp,
    updatedAt:
      timestamps?.updatedAt ??
      timestamps?.finishedAt ??
      timestamps?.startedAt ??
      fallbackTimestamp,
  };
}

function phaseNamesFromDurable(
  durable: FactorySessionDurableReadModel,
): string[] {
  const phases = (durable.phaseSummaries ?? [])
    .map((summary) => summary.phase.trim())
    .filter((phase) => phase.length > 0);

  if (phases.length > 0) {
    return phases;
  }

  return durable.phase ? [durable.phase] : [];
}

function childDispatchCountsFromDurableProgress(
  progress?: components["schemas"]["FactorySessionDurableProgressCounts"],
): components["schemas"]["FactorySessionJavaScriptChildDispatchCounts"] {
  const total = progress?.totalDispatches ?? 0;
  const completed = progress?.completedDispatches ?? 0;
  const inFlight =
    progress?.runningDispatches ?? progress?.inFlightDispatches ?? 0;
  const failed = progress?.failedDispatches ?? 0;
  const queued =
    progress?.queuedDispatches ??
    Math.max(total - completed - inFlight - failed, 0);

  return {
    completed,
    queued,
    running: inFlight,
  };
}

function artifactRefsToArtifacts(
  artifactRefs?: components["schemas"]["FactoryArtifactRef"][],
): components["schemas"]["FactoryArtifact"][] | undefined {
  if (!artifactRefs || artifactRefs.length === 0) {
    return undefined;
  }

  return artifactRefs.map((artifactRef) => ({
    id: artifactRef.id,
    kind: artifactRef.kind,
    visibility: artifactRef.visibility,
  }));
}

export function runtimeStatusFromDurableLifecycle(
  status: components["schemas"]["FactorySessionDurableLifecycleStatus"],
): components["schemas"]["FactorySessionStatus"] {
  switch (status) {
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusRunning:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusResuming:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusCanceling:
      return FactorySessionStatus.ACTIVE;
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusQueued:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusAwaitingApproval:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusPaused:
      return FactorySessionStatus.IDLE;
    default:
      return FactorySessionStatus.FINISHED;
  }
}

export function scriptStatusFromDurableLifecycle(
  status: components["schemas"]["FactorySessionDurableLifecycleStatus"],
): components["schemas"]["FactorySessionJavaScriptScriptStatus"] {
  switch (status) {
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusRunning:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusResuming:
      return FactorySessionJavaScriptScriptStatus.RUNNING;
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusPaused:
      return FactorySessionJavaScriptScriptStatus.PAUSED;
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusFailed:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusTimedOut:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusInterrupted:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusTerminated:
      return FactorySessionJavaScriptScriptStatus.FAILED;
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusSucceeded:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusCanceled:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusCanceling:
      return FactorySessionJavaScriptScriptStatus.FINISHED;
    default:
      return FactorySessionJavaScriptScriptStatus.IDLE;
  }
}

function isFactorySession(value: unknown): value is FactorySession {
  return (
    isAPIRecord(value) &&
    typeof value.id === "string" &&
    isAPIRecord(value.runtime) &&
    typeof value.runtime.orchestratorKind === "string"
  );
}

function isFactorySessionDurableReadModel(
  value: unknown,
): value is FactorySessionDurableReadModel {
  return (
    isAPIRecord(value) &&
    typeof value.sessionId === "string" &&
    typeof value.status === "string" &&
    typeof value.orchestratorKind === "string" &&
    isAPIRecord(value.resolvedSource)
  );
}

function isAPIRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

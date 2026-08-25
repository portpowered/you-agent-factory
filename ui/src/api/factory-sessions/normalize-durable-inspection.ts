import type { components } from "../generated/openapi";
import {
  ConfirmationState,
  FactoryOrchestratorKind,
  FactorySessionDurableLifecycleStatus,
  FactorySessionResultStatus,
  FactorySessionStatus,
} from "../generated/openapi";

type FactoryArtifactRef = components["schemas"]["FactoryArtifactRef"];
type FactoryDispatch = components["schemas"]["FactoryDispatch"];
type FactorySessionDispatchSummary =
  components["schemas"]["FactorySessionDispatchSummary"];
type FactorySessionDurableReadModel =
  components["schemas"]["FactorySessionDurableReadModel"];
type FactorySessionLiveResult =
  components["schemas"]["FactorySessionLiveResult"];
type FactorySessionPartialResult =
  components["schemas"]["FactorySessionPartialResult"];
type FactorySessionResult = components["schemas"]["FactorySessionResult"];

export interface DurableResultSurfaces {
  partialResult?: FactorySessionPartialResult;
  result?: FactorySessionLiveResult;
}

export function resultSurfacesFromDurableReadModel(
  durable: FactorySessionDurableReadModel,
): DurableResultSurfaces {
  const artifactRefs = collectDurableArtifactRefs(durable);
  if (artifactRefs.length === 0) {
    return {};
  }

  const resultStatus = durable.resultSummary?.resultStatus;
  if (
    resultStatus === FactorySessionResultStatus.FactorySessionResultStatusFinal
  ) {
    const resultArtifactRef = pickFinalArtifactRef(artifactRefs);
    if (!resultArtifactRef) {
      return {};
    }
    return {
      result: {
        resultArtifactRef,
        sessionId: durable.sessionId,
        status: FactorySessionStatus.FINISHED,
      },
    };
  }

  if (
    resultStatus ===
      FactorySessionResultStatus.FactorySessionResultStatusPartial ||
    resultStatus ===
      FactorySessionResultStatus.FactorySessionResultStatusFailedWithPartial
  ) {
    const partialResultArtifactRef = artifactRefs[0];
    if (!partialResultArtifactRef) {
      return {};
    }
    return {
      partialResult: {
        partialResultArtifactRef,
        phase: durable.phase ?? "",
        sessionId: durable.sessionId,
      },
    };
  }

  return {};
}

export function resultSurfacesFromDurableResult(
  durableResult: FactorySessionResult,
  fallbackPhase?: string,
): DurableResultSurfaces {
  const artifactRefs = durableResult.artifactRefs ?? [];
  if (artifactRefs.length === 0) {
    return {};
  }

  if (
    durableResult.resultStatus ===
    FactorySessionResultStatus.FactorySessionResultStatusFinal
  ) {
    const resultArtifactRef = pickFinalArtifactRef(artifactRefs);
    if (!resultArtifactRef) {
      return {};
    }
    return {
      result: {
        resultArtifactRef,
        sessionId: durableResult.sessionId,
        status: FactorySessionStatus.FINISHED,
      },
    };
  }

  if (
    durableResult.resultStatus ===
      FactorySessionResultStatus.FactorySessionResultStatusPartial ||
    durableResult.resultStatus ===
      FactorySessionResultStatus.FactorySessionResultStatusFailedWithPartial
  ) {
    return {
      partialResult: {
        partialResultArtifactRef: artifactRefs[0],
        phase: readPhaseFromDurableResult(durableResult) ?? fallbackPhase ?? "",
        sessionId: durableResult.sessionId,
      },
    };
  }

  return {};
}

export function dispatchSummariesToFactoryDispatches(
  sessionId: string,
  orchestratorKind: components["schemas"]["FactoryOrchestratorKind"],
  summaries: FactorySessionDispatchSummary[],
): FactoryDispatch[] {
  return summaries.map((summary) => ({
    artifactIds: summary.outputArtifactIds,
    attempt: summary.attempt,
    confirmationState:
      summary.confirmationState ?? ConfirmationState.UNCONFIRMED,
    dispatchKind: summary.dispatchKind,
    failureDetail: summary.failureDetail,
    id: summary.id,
    label: summary.label,
    model: summary.model,
    orchestratorKind,
    phase: summary.phase,
    provider: summary.provider,
    providerSessionRefs: summary.providerSessionRefs,
    runnerId: summary.runnerId,
    sessionId,
    status: summary.status,
    usage: summary.usage,
    warnings: summary.warnings,
    javascript: summary.javascript,
  }));
}

function collectDurableArtifactRefs(
  durable: FactorySessionDurableReadModel,
): FactoryArtifactRef[] {
  if (durable.artifactRefs && durable.artifactRefs.length > 0) {
    return durable.artifactRefs;
  }
  return durable.resultSummary?.artifactRefs ?? [];
}

function pickFinalArtifactRef(
  artifactRefs: FactoryArtifactRef[],
): FactoryArtifactRef | undefined {
  return (
    artifactRefs.find((artifactRef) => artifactRef.kind === "FINAL_RESULT") ??
    artifactRefs[0]
  );
}

function readPhaseFromDurableResult(
  durableResult: FactorySessionResult,
): string | undefined {
  const primaryResult = durableResult.primaryResult;
  if (!Array.isArray(primaryResult)) {
    return undefined;
  }

  for (const part of primaryResult) {
    if (
      typeof part === "object" &&
      part !== null &&
      "json" in part &&
      typeof part.json === "object" &&
      part.json !== null &&
      "phase" in part.json &&
      typeof part.json.phase === "string"
    ) {
      return part.json.phase;
    }
  }

  return undefined;
}

export function isDurableJavaScriptSession(
  sessionId: string,
  orchestratorKind: components["schemas"]["FactoryOrchestratorKind"],
  durableLifecycleStatus?: components["schemas"]["FactorySessionDurableLifecycleStatus"],
): boolean {
  return (
    durableLifecycleStatus !== undefined &&
    orchestratorKind === FactoryOrchestratorKind.JAVASCRIPT &&
    sessionId.startsWith("dur-sess-")
  );
}

export interface DurableSupplementalReadPlan {
  fetchDispatches: boolean;
  fetchFinalResults: boolean;
  fetchPartialResults: boolean;
}

export function durableSupplementalReadPlan(input: {
  durableLifecycleStatus?: FactorySessionDurableReadModel["status"];
  partialResult?: FactorySessionPartialResult;
  progress?: FactorySessionDurableReadModel["progress"];
  result?: FactorySessionLiveResult;
  resultSummary?: FactorySessionDurableReadModel["resultSummary"];
}): DurableSupplementalReadPlan {
  return {
    fetchDispatches: shouldFetchDurableDispatches(input),
    fetchFinalResults: shouldFetchDurableFinalResults(input),
    fetchPartialResults: shouldFetchDurablePartialResults(input),
  };
}

export function shouldFetchDurableDispatches(input: {
  durableLifecycleStatus?: FactorySessionDurableReadModel["status"];
  progress?: FactorySessionDurableReadModel["progress"];
}): boolean {
  const totalDispatches = input.progress?.totalDispatches ?? 0;
  if (totalDispatches === 0) {
    return false;
  }

  if (isInspectableDurableLifecycle(input.durableLifecycleStatus)) {
    return true;
  }

  const hasActiveDispatch = (input.progress?.inFlightDispatches ?? 0) > 0;
  return (
    hasActiveDispatch &&
    (input.durableLifecycleStatus === "RUNNING" ||
      input.durableLifecycleStatus === "PAUSED" ||
      input.durableLifecycleStatus === "RESUMING")
  );
}

export function shouldFetchDurableFinalResults(input: {
  durableLifecycleStatus?: FactorySessionDurableReadModel["status"];
  result?: FactorySessionLiveResult;
  resultSummary?: FactorySessionDurableReadModel["resultSummary"];
}): boolean {
  if (input.result) {
    return false;
  }

  if (!isInspectableDurableLifecycle(input.durableLifecycleStatus)) {
    return false;
  }

  const resultStatus = input.resultSummary?.resultStatus;
  if (
    resultStatus ===
      FactorySessionResultStatus.FactorySessionResultStatusPartial ||
    resultStatus ===
      FactorySessionResultStatus.FactorySessionResultStatusFailedWithPartial
  ) {
    return false;
  }

  return true;
}

export function shouldFetchDurablePartialResults(input: {
  partialResult?: FactorySessionPartialResult;
  result?: FactorySessionLiveResult;
  resultSummary?: FactorySessionDurableReadModel["resultSummary"];
}): boolean {
  if (input.partialResult || input.result) {
    return false;
  }

  const resultStatus = input.resultSummary?.resultStatus;
  if (
    resultStatus === FactorySessionResultStatus.FactorySessionResultStatusFinal
  ) {
    return false;
  }

  return (
    resultStatus ===
      FactorySessionResultStatus.FactorySessionResultStatusPartial ||
    resultStatus ===
      FactorySessionResultStatus.FactorySessionResultStatusFailedWithPartial
  );
}

function isInspectableDurableLifecycle(
  status?: FactorySessionDurableReadModel["status"],
): boolean {
  if (!status) {
    return false;
  }

  switch (status) {
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusSucceeded:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusFailed:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusCanceled:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusTimedOut:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusInterrupted:
    case FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusTerminated:
      return true;
    default:
      return false;
  }
}

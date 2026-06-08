import type { FactoryEvent } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import type {
  ReplayJavaScriptDispatch,
  ReplayJavaScriptRuntime,
  ReplaySessionArtifact,
  ReplayWorldState,
} from "./types";

function stringValue(value: string | null | undefined): string | undefined {
  if (value == null) {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

function dispatchLifecyclePhase(context: FactoryEvent["context"]): string | undefined {
  return stringValue(context.phaseName) ?? stringValue(context.phaseId);
}

function ensureJavaScriptRuntime(state: ReplayWorldState): ReplayJavaScriptRuntime {
  if (!state.javascriptRuntime) {
    state.javascriptRuntime = {
      checkpoints: [],
      child_dispatch_counts: { completed: 0, queued: 0, running: 0 },
      dispatches: [],
      phases: [],
    };
  }
  return state.javascriptRuntime;
}

function mergeJavaScriptDispatch(
  existing: ReplayJavaScriptDispatch,
  incoming: ReplayJavaScriptDispatch,
): ReplayJavaScriptDispatch {
  return {
    ...existing,
    artifact_ids: incoming.artifact_ids ?? existing.artifact_ids,
    dispatch_kind: incoming.dispatch_kind ?? existing.dispatch_kind,
    label: incoming.label ?? existing.label,
    phase: incoming.phase ?? existing.phase,
    status: incoming.status || existing.status,
  };
}

function upsertJavaScriptDispatch(
  runtime: ReplayJavaScriptRuntime,
  dispatch: ReplayJavaScriptDispatch,
): void {
  const index = runtime.dispatches.findIndex((entry) => entry.id === dispatch.id);
  if (index >= 0) {
    runtime.dispatches[index] = mergeJavaScriptDispatch(
      runtime.dispatches[index],
      dispatch,
    );
  } else {
    runtime.dispatches = [...runtime.dispatches, dispatch];
  }
  recountJavaScriptDispatchTotals(runtime);
}

function recountJavaScriptDispatchTotals(runtime: ReplayJavaScriptRuntime): void {
  let queued = 0;
  let running = 0;
  let completed = 0;
  for (const dispatch of runtime.dispatches) {
    switch (dispatch.status) {
      case "QUEUED":
        queued += 1;
        break;
      case "RUNNING":
        running += 1;
        break;
      case "COMPLETED":
      case "RECONCILED":
        completed += 1;
        break;
      default:
        break;
    }
  }
  runtime.child_dispatch_counts = { completed, queued, running };
}

function applyDispatchQueued(state: ReplayWorldState, event: FactoryEvent): void {
  const payload = event.payload as Record<string, unknown>;
  const dispatchID = stringValue(event.context.dispatchId);
  if (!dispatchID) {
    return;
  }
  const runtime = ensureJavaScriptRuntime(state);
  upsertJavaScriptDispatch(runtime, {
    dispatch_kind: stringValue(payload.dispatchKind as string | undefined),
    id: dispatchID,
    label: stringValue(payload.label as string | undefined),
    phase: dispatchLifecyclePhase(event.context),
    status: "QUEUED",
  });
}

function applyDispatchInterrupted(
  state: ReplayWorldState,
  event: FactoryEvent,
): void {
  const payload = event.payload as Record<string, unknown>;
  const dispatchID = stringValue(event.context.dispatchId);
  if (!dispatchID) {
    return;
  }
  const runtime = ensureJavaScriptRuntime(state);
  upsertJavaScriptDispatch(runtime, {
    id: dispatchID,
    phase: dispatchLifecyclePhase(event.context),
    status:
      stringValue(payload.observedStatus as string | undefined) ?? "INTERRUPTED",
  });
}

function applyDispatchReconciled(
  state: ReplayWorldState,
  event: FactoryEvent,
): void {
  const payload = event.payload as Record<string, unknown>;
  const dispatchID = stringValue(event.context.dispatchId);
  if (!dispatchID) {
    return;
  }
  const runtime = ensureJavaScriptRuntime(state);
  const artifactIDs = [...(payload.artifactIds as string[] | undefined ?? [])];
  const resultArtifactRef = payload.resultArtifactRef as { id?: string } | undefined;
  if (resultArtifactRef?.id) {
    artifactIDs.push(resultArtifactRef.id);
  }
  upsertJavaScriptDispatch(runtime, {
    artifact_ids: artifactIDs.length > 0 ? artifactIDs : undefined,
    id: dispatchID,
    phase: dispatchLifecyclePhase(event.context),
    status:
      stringValue(payload.reconciledStatus as string | undefined) ?? "RECONCILED",
  });
}

function applyArtifactCreated(state: ReplayWorldState, event: FactoryEvent): void {
  const payload = event.payload as Record<string, unknown>;
  const artifactID = stringValue(payload.artifactId as string | undefined);
  if (!artifactID) {
    return;
  }
  const artifact: ReplaySessionArtifact = {
    content_type: stringValue(payload.contentType as string | undefined),
    id: artifactID,
    kind: stringValue(payload.kind as string | undefined),
    label: stringValue(payload.label as string | undefined),
    visibility: stringValue(payload.visibility as string | undefined),
  };
  state.sessionArtifacts = [...state.sessionArtifacts, artifact];
}

export function applyDispatchLifecycleEvent(
  state: ReplayWorldState,
  event: FactoryEvent,
): boolean {
  switch (event.type) {
    case FACTORY_EVENT_TYPES.dispatchQueued:
      applyDispatchQueued(state, event);
      return true;
    case FACTORY_EVENT_TYPES.dispatchInterrupted:
      applyDispatchInterrupted(state, event);
      return true;
    case FACTORY_EVENT_TYPES.dispatchReconciled:
      applyDispatchReconciled(state, event);
      return true;
    case FACTORY_EVENT_TYPES.artifactCreated:
      applyArtifactCreated(state, event);
      return true;
    default:
      return false;
  }
}

import type { FactoryEvent } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import { FactorySessionJavaScriptScriptStatus } from "../../../../api/generated/openapi";
import type { ReplayJavaScriptRuntime, ReplayWorldState } from "./types";

function stringValue(value: string | null | undefined): string | undefined {
  if (value == null) {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

function ensureJavaScriptRuntime(
  state: ReplayWorldState,
): ReplayJavaScriptRuntime {
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

function orchestratorPhaseHistoryName(
  name: string | undefined,
  id: string | undefined,
): string | undefined {
  return stringValue(name) ?? stringValue(id);
}

function appendPhaseHistoryEntry(phases: string[], phase: string): string[] {
  if (phase === "") {
    return phases;
  }
  if (phases.length > 0 && phases[phases.length - 1] === phase) {
    return phases;
  }
  return [...phases, phase];
}

function orchestratorPhaseStatusToScriptStatus(status: string): string {
  switch (status) {
    case "ACTIVE":
      return FactorySessionJavaScriptScriptStatus.RUNNING;
    case "COMPLETED":
      return FactorySessionJavaScriptScriptStatus.FINISHED;
    case "SKIPPED":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "SKIPPED";
    default:
      return status;
  }
}

function applyOrchestratorPhaseChanged(
  state: ReplayWorldState,
  event: FactoryEvent,
): void {
  const payload = event.payload as Record<string, unknown>;
  const runtime = ensureJavaScriptRuntime(state);
  const currentPhase =
    stringValue(event.context.phaseName) ??
    stringValue(event.context.phaseId) ??
    "";
  if (currentPhase) {
    runtime.phase = currentPhase;
  }
  const previous = orchestratorPhaseHistoryName(
    payload.previousPhaseName as string | undefined,
    payload.previousPhaseId as string | undefined,
  );
  if (previous) {
    runtime.phases = appendPhaseHistoryEntry(runtime.phases, previous);
  }
  if (currentPhase) {
    runtime.phases = appendPhaseHistoryEntry(runtime.phases, currentPhase);
  }
  if (typeof payload.phaseStatus === "string") {
    runtime.script_status = orchestratorPhaseStatusToScriptStatus(
      payload.phaseStatus,
    );
  }
}

function applyOrchestratorCheckpointWritten(
  state: ReplayWorldState,
  event: FactoryEvent,
): void {
  const payload = event.payload as Record<string, unknown>;
  const runtime = ensureJavaScriptRuntime(state);
  const artifactRef = payload.artifactRef as { id?: string } | undefined;
  const checkpointID =
    stringValue(event.context.checkpointId) ?? stringValue(artifactRef?.id);
  if (!checkpointID) {
    return;
  }
  const checkpoint = {
    id: checkpointID,
    label: stringValue(payload.label as string | undefined),
    summary: stringValue(
      (payload.summary as string | undefined) ??
        (payload.phaseSummary as string | undefined),
    ),
  };
  runtime.checkpoints = [...runtime.checkpoints, checkpoint];
}

export function applyOrchestratorProgressEvent(
  state: ReplayWorldState,
  event: FactoryEvent,
): boolean {
  switch (event.type) {
    case FACTORY_EVENT_TYPES.orchestratorPhaseChanged:
      applyOrchestratorPhaseChanged(state, event);
      return true;
    case FACTORY_EVENT_TYPES.orchestratorCheckpointWritten:
      applyOrchestratorCheckpointWritten(state, event);
      return true;
    default:
      return false;
  }
}

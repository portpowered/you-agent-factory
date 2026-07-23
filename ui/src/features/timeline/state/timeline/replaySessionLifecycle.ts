import type { DashboardSessionBracket } from "../../../../api/dashboard";
import type { FactoryEvent } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import type { ReplayWorldState } from "./types";

function stringValue(value: string | null | undefined): string | undefined {
  if (value == null) {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

function ensureSessionBracket(
  state: ReplayWorldState,
): DashboardSessionBracket {
  if (!state.sessionBracket) {
    state.sessionBracket = {};
  }
  return state.sessionBracket;
}

function mergeSessionBracketIdentity(
  bracket: DashboardSessionBracket,
  context: FactoryEvent["context"],
): void {
  const sessionID = stringValue(context.sessionId);
  if (sessionID) {
    bracket.session_id = sessionID;
  }
  if (context.orchestratorKind && !bracket.orchestrator_kind) {
    bracket.orchestrator_kind = context.orchestratorKind;
  }
  const dialect = stringValue(context.orchestratorDialect);
  if (dialect && !bracket.orchestrator_dialect) {
    bracket.orchestrator_dialect = dialect;
  }
}

function cloneStringSlice(
  values: string[] | null | undefined,
): string[] | undefined {
  if (!values || values.length === 0) {
    return undefined;
  }
  return [...values];
}

function resultSummaryFromPayload(
  payload: Record<string, unknown>,
): DashboardSessionBracket["result_summary"] {
  const summary = payload.resultSummary;
  if (!Array.isArray(summary)) {
    return undefined;
  }
  const projected: NonNullable<DashboardSessionBracket["result_summary"]> = [];
  for (const part of summary) {
    if (part == null || typeof part !== "object") {
      continue;
    }
    const typed = part as { text?: string; type?: string };
    projected.push({
      text: typed.text,
      type: typed.type,
    });
  }
  return projected.length > 0 ? projected : undefined;
}

function applySessionStarted(
  state: ReplayWorldState,
  event: FactoryEvent,
): void {
  const payload = event.payload as Record<string, unknown>;
  const bracket = ensureSessionBracket(state);
  mergeSessionBracketIdentity(bracket, event.context);
  bracket.factory_id = stringValue(payload.factoryId as string | undefined);
  bracket.source_ref = stringValue(payload.sourceRef as string | undefined);
  if (typeof payload.startedAt === "string") {
    bracket.started_at = payload.startedAt;
  }
}

function applySessionResultUpdated(
  state: ReplayWorldState,
  event: FactoryEvent,
): void {
  const payload = event.payload as Record<string, unknown>;
  const bracket = ensureSessionBracket(state);
  mergeSessionBracketIdentity(bracket, event.context);
  if (typeof payload.resultStatus === "string") {
    bracket.result_status = payload.resultStatus;
  }
  bracket.result_summary = resultSummaryFromPayload(payload);
  bracket.artifact_ids = cloneStringSlice(
    payload.artifactIds as string[] | undefined,
  );
}

function applySessionPaused(
  state: ReplayWorldState,
  event: FactoryEvent,
): void {
  const payload = event.payload as Record<string, unknown>;
  const bracket = ensureSessionBracket(state);
  mergeSessionBracketIdentity(bracket, event.context);
  if (typeof payload.status === "string") {
    bracket.lifecycle_control_status = payload.status;
  }
  if (typeof payload.pausedAt === "string") {
    bracket.paused_at = payload.pausedAt;
  }
}

function applySessionResumed(
  state: ReplayWorldState,
  event: FactoryEvent,
): void {
  const payload = event.payload as Record<string, unknown>;
  const bracket = ensureSessionBracket(state);
  mergeSessionBracketIdentity(bracket, event.context);
  if (typeof payload.status === "string") {
    bracket.lifecycle_control_status = payload.status;
  }
  if (typeof payload.resumedAt === "string") {
    bracket.resumed_at = payload.resumedAt;
  }
}

function applySessionLifecycleControl(
  state: ReplayWorldState,
  event: FactoryEvent,
): void {
  const payload = event.payload as Record<string, unknown>;
  if (payload.outcome !== "ACCEPTED") {
    return;
  }
  const bracket = ensureSessionBracket(state);
  mergeSessionBracketIdentity(bracket, event.context);
  if (typeof payload.newStatus === "string") {
    bracket.lifecycle_control_status = payload.newStatus;
  }
  if (typeof payload.occurredAt !== "string") {
    return;
  }
  const operation = payload.operation;
  if (operation === "PAUSE") {
    bracket.paused_at = payload.occurredAt;
    return;
  }
  if (operation === "RESUME") {
    bracket.resumed_at = payload.occurredAt;
  }
}

function applySessionCompleted(
  state: ReplayWorldState,
  event: FactoryEvent,
): void {
  const payload = event.payload as Record<string, unknown>;
  const bracket = ensureSessionBracket(state);
  mergeSessionBracketIdentity(bracket, event.context);
  bracket.terminal = true;
  if (typeof payload.finalStatus === "string") {
    bracket.final_status = payload.finalStatus;
  }
  if (typeof payload.completedAt === "string") {
    bracket.completed_at = payload.completedAt;
  }
  if (typeof payload.durationMillis === "number") {
    bracket.duration_millis = payload.durationMillis;
  }
  if (typeof payload.resultStatus === "string") {
    bracket.result_status = payload.resultStatus;
  }
  bracket.artifact_ids = cloneStringSlice(
    payload.artifactIds as string[] | undefined,
  );
  const dispatchCounts = payload.dispatchCounts as
    | { completed?: number; queued?: number; running?: number }
    | undefined;
  if (dispatchCounts) {
    bracket.dispatch_counts = {
      completed: dispatchCounts.completed ?? 0,
      queued: dispatchCounts.queued ?? 0,
      running: dispatchCounts.running ?? 0,
    };
  }
  const failureDetail = payload.failureDetail as
    | { message?: string; reason?: string }
    | undefined;
  if (failureDetail) {
    bracket.failure_reason = stringValue(failureDetail.reason);
    bracket.failure_message = stringValue(failureDetail.message);
  }
}

export function applySessionLifecycleEvent(
  state: ReplayWorldState,
  event: FactoryEvent,
): boolean {
  switch (event.type) {
    case FACTORY_EVENT_TYPES.sessionStarted:
      applySessionStarted(state, event);
      return true;
    case FACTORY_EVENT_TYPES.sessionPaused:
      applySessionPaused(state, event);
      return true;
    case FACTORY_EVENT_TYPES.sessionResumed:
      applySessionResumed(state, event);
      return true;
    case FACTORY_EVENT_TYPES.sessionLifecycleControl:
      applySessionLifecycleControl(state, event);
      return true;
    case FACTORY_EVENT_TYPES.sessionResultUpdated:
      applySessionResultUpdated(state, event);
      return true;
    case FACTORY_EVENT_TYPES.sessionCompleted:
      applySessionCompleted(state, event);
      return true;
    default:
      return false;
  }
}

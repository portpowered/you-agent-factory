import type { DashboardTraceDispatch } from "../../../api/dashboard/types";

/**
 * The smallest identity that can distinguish one trace execution from every
 * other execution shown by the drilldown.
 *
 * React Flow node ids and table keys are projections of this value. They must
 * not become a second source of trace data or drop the attempt dimension.
 */
export interface TraceSelectionIdentity {
  attempt: number;
  dispatch_id: string;
  work_id: string;
}

// Older recordings and non-provider dispatches do not carry an inference
// attempt. Replay supplies the field for current provider-backed dispatches;
// retain one only as the compatibility identity for those legacy records.
const LEGACY_TRACE_SELECTION_ATTEMPT = 1;

export function traceSelectionKey(identity: TraceSelectionIdentity): string {
  return [
    encodeURIComponent(identity.dispatch_id),
    encodeURIComponent(identity.work_id),
    String(identity.attempt),
  ].join("|");
}

export function traceSelectionMatches(
  left: TraceSelectionIdentity | null | undefined,
  right: TraceSelectionIdentity | null | undefined,
): boolean {
  return (
    left !== null &&
    left !== undefined &&
    right !== null &&
    right !== undefined &&
    left.dispatch_id === right.dispatch_id &&
    left.work_id === right.work_id &&
    left.attempt === right.attempt
  );
}

export function traceSelectionAttempt(
  dispatch: Pick<DashboardTraceDispatch, "attempt">,
): number {
  const attempt = dispatch.attempt;
  return typeof attempt === "number" && Number.isInteger(attempt) && attempt > 0
    ? attempt
    : LEGACY_TRACE_SELECTION_ATTEMPT;
}

export function traceDispatchWorkIDs(
  dispatch: Pick<
    DashboardTraceDispatch,
    "input_items" | "output_items" | "work_ids"
  >,
): string[] {
  const workIDs = new Set<string>();

  for (const workID of dispatch.work_ids ?? []) {
    addWorkID(workIDs, workID);
  }
  for (const workItem of dispatch.input_items ?? []) {
    addWorkID(workIDs, workItem.work_id);
  }
  for (const workItem of dispatch.output_items ?? []) {
    addWorkID(workIDs, workItem.work_id);
  }

  return [...workIDs];
}

export function traceSelectionForDispatch(
  dispatch: Pick<DashboardTraceDispatch, "attempt" | "dispatch_id"> &
    Partial<
      Pick<DashboardTraceDispatch, "input_items" | "output_items" | "work_ids">
    >,
  workID?: string,
): TraceSelectionIdentity {
  return {
    attempt: traceSelectionAttempt(dispatch),
    dispatch_id: dispatch.dispatch_id,
    work_id: workID ?? traceDispatchWorkIDs(dispatch)[0] ?? "",
  };
}

export function traceSelectionIdentitiesForDispatch(
  dispatch: Pick<DashboardTraceDispatch, "attempt" | "dispatch_id"> &
    Partial<
      Pick<DashboardTraceDispatch, "input_items" | "output_items" | "work_ids">
    >,
): TraceSelectionIdentity[] {
  const workIDs = traceDispatchWorkIDs(dispatch);
  return (workIDs.length > 0 ? workIDs : [""]).map((workID) =>
    traceSelectionForDispatch(dispatch, workID),
  );
}

export function traceSelectionIdentitiesByWorkID(
  dispatches: readonly DashboardTraceDispatch[],
): ReadonlyMap<string, readonly TraceSelectionIdentity[]> {
  const selectionsByWorkID = new Map<string, TraceSelectionIdentity[]>();

  for (const dispatch of dispatches) {
    for (const selection of traceSelectionIdentitiesForDispatch(dispatch)) {
      if (!selection.work_id) {
        continue;
      }

      const selections = selectionsByWorkID.get(selection.work_id) ?? [];
      if (
        !selections.some((candidate) =>
          traceSelectionMatches(candidate, selection),
        )
      ) {
        selections.push(selection);
        selectionsByWorkID.set(selection.work_id, selections);
      }
    }
  }

  return selectionsByWorkID;
}

function addWorkID(workIDs: Set<string>, workID: string | undefined): void {
  const normalized = workID?.trim();
  if (normalized) {
    workIDs.add(normalized);
  }
}

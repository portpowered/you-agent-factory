import type {
  DashboardTraceToken,
  DashboardWorkItemRef,
} from "../../../../api/dashboard";
import type { FactoryWorkItem } from "../../../../api/events";
import { uniqueSorted } from "./shared";
import { dashboardWorkTypeID, isSystemTimeWorkItem } from "./systemTime";
import {
  cloneLineageWorkItem,
  resolveConsumedInputSnapshot,
  resolveOutputWorkSnapshot,
  resolveSelectedWorkSnapshot,
  type WorkPayloadLineageProjection,
  type WorkPayloadSnapshot,
} from "./workPayloadLineage";

export function workRef(item: FactoryWorkItem): DashboardWorkItemRef {
  return workItemRef(item);
}

export function workItemRef(item: FactoryWorkItem): DashboardWorkItemRef {
  const currentChainingTraceID =
    item.current_chaining_trace_id ?? item.trace_id;
  return {
    ...(currentChainingTraceID
      ? { current_chaining_trace_id: currentChainingTraceID }
      : {}),
    display_name: item.display_name,
    ...(item.previous_chaining_trace_ids
      ? {
          previous_chaining_trace_ids: [...item.previous_chaining_trace_ids],
        }
      : {}),
    ...(item.state ? { state: item.state } : {}),
    trace_id: item.trace_id,
    work_id: item.id,
    work_type_id: dashboardWorkTypeID(item.work_type_id),
  };
}

export function workItemRefWithSelectedPayload(
  lineage: WorkPayloadLineageProjection,
  item: FactoryWorkItem,
): DashboardWorkItemRef {
  return selectedWorkItemRefForID(lineage, item.id, item);
}

export function selectedWorkItemRefForID(
  lineage: WorkPayloadLineageProjection,
  workID: string,
  fallback?: FactoryWorkItem,
): DashboardWorkItemRef {
  const resolution = resolveSelectedWorkSnapshot(lineage, workID);
  if (resolution.status === "RESOLVED" && resolution.snapshot) {
    return lineageResolvedWorkItemRef(resolution.snapshot, resolution.status);
  }

  const item: FactoryWorkItem =
    fallback ??
    ({
      id: workID,
      work_type_id: "",
    } satisfies FactoryWorkItem);
  const ref = workItemRef(item);
  if (!ref.work_id) {
    ref.work_id = workID;
  }
  if (item.state) {
    ref.state = item.state;
  }
  ref.payload_status = resolution.status;
  ref.payload_unavailable_reason = resolution.reason;
  return ref;
}

export function workItemRefWithConsumedPayload(
  lineage: WorkPayloadLineageProjection,
  dispatchID: string,
  item: FactoryWorkItem,
): DashboardWorkItemRef {
  return consumedWorkItemRefForID(lineage, dispatchID, item.id, item);
}

export function workItemRefWithOutputPayload(
  lineage: WorkPayloadLineageProjection,
  dispatchID: string,
  item: FactoryWorkItem,
): DashboardWorkItemRef {
  const resolution = resolveOutputWorkSnapshot(lineage, dispatchID, item.id);
  if (resolution.status === "RESOLVED" && resolution.snapshot) {
    return lineageResolvedWorkItemRef(resolution.snapshot, resolution.status);
  }

  const ref = workItemRef(item);
  ref.payload_status = resolution.status;
  ref.payload_unavailable_reason = resolution.reason;
  return ref;
}

export function workItemRefsForIDs(
  lineage: WorkPayloadLineageProjection,
  ids: string[],
  itemsByID: Record<string, FactoryWorkItem>,
): DashboardWorkItemRef[] {
  const refs: DashboardWorkItemRef[] = [];
  for (const id of [...ids].sort()) {
    const item = itemsByID[id];
    if (!item?.id || isSystemTimeWorkItem(item)) {
      continue;
    }
    refs.push(workItemRefWithSelectedPayload(lineage, item));
  }
  return refs;
}

function consumedWorkItemRefForID(
  lineage: WorkPayloadLineageProjection,
  dispatchID: string,
  workID: string,
  fallback?: FactoryWorkItem,
): DashboardWorkItemRef {
  const resolution = resolveConsumedInputSnapshot(lineage, dispatchID, workID);
  if (resolution.status === "RESOLVED" && resolution.snapshot) {
    return lineageResolvedWorkItemRef(resolution.snapshot, resolution.status);
  }

  const item: FactoryWorkItem =
    fallback ??
    ({
      id: workID,
      work_type_id: "",
    } satisfies FactoryWorkItem);
  const ref = workItemRef(item);
  if (!ref.work_id) {
    ref.work_id = workID;
  }
  if (item.state) {
    ref.state = item.state;
  }
  ref.payload_status = resolution.status;
  ref.payload_unavailable_reason = resolution.reason;
  return ref;
}

export function consumedWorkItemRefsForDispatch(
  lineage: WorkPayloadLineageProjection,
  dispatchID: string,
  consumedTokens: DashboardTraceToken[],
  workItemsByID: Record<string, FactoryWorkItem>,
): DashboardWorkItemRef[] {
  const seen = new Set<string>();
  const refs: DashboardWorkItemRef[] = [];
  for (const workID of uniqueSorted(
    consumedTokens
      .map((token) => token.work_id)
      .filter((workID): workID is string => Boolean(workID)),
  )) {
    if (seen.has(workID)) {
      continue;
    }
    const item = workItemsByID[workID];
    if (!item || isSystemTimeWorkItem(item)) {
      continue;
    }
    seen.add(workID);
    refs.push(workItemRefWithConsumedPayload(lineage, dispatchID, item));
  }
  return refs.sort((left, right) => left.work_id.localeCompare(right.work_id));
}

function lineageResolvedWorkItemRef(
  snapshot: WorkPayloadSnapshot,
  payloadStatus: string,
): DashboardWorkItemRef {
  const item = cloneLineageWorkItem(snapshot.work_item);
  const ref = workItemRef(item);
  ref.state = item.state;
  ref.content = item.content;
  ref.payload_status = payloadStatus;
  ref.lineage_logical_work_id = snapshot.logical_work_id;
  ref.lineage_source_kind = snapshot.source_kind;
  ref.lineage_continuity = snapshot.continuity;
  ref.lineage_parent_work_ids = snapshot.parent_work_ids
    ? [...snapshot.parent_work_ids]
    : undefined;
  return ref;
}

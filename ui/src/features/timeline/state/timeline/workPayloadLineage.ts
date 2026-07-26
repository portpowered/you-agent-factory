import type { FactoryWorkItem } from "../../../../api/events";
import type { WorkContent } from "../../../work-content/lib/work-content-types";

export type WorkPayloadSnapshotKind =
  | "WORK_REQUEST"
  | "DISPATCH_RESPONSE_OUTPUT";

export type WorkPayloadContinuity =
  | "INITIAL_SUBMISSION"
  | "SAME_WORK_ID_CONTINUATION"
  | "NEW_DOWNSTREAM_WORK";

export type WorkPayloadResolutionStatus = "RESOLVED" | "UNAVAILABLE";

export interface WorkPayloadSnapshot {
  snapshot_id: string;
  work_id: string;
  logical_work_id: string;
  source_kind: WorkPayloadSnapshotKind;
  source_event_type: string;
  request_id?: string;
  dispatch_id?: string;
  observed_tick?: number;
  continuity: WorkPayloadContinuity;
  parent_snapshot_ids?: string[];
  parent_work_ids?: string[];
  parent_logical_work_ids?: string[];
  work_item: FactoryWorkItem;
}

export interface WorkPayloadRef {
  status: WorkPayloadResolutionStatus;
  snapshot_id?: string;
  reason?: string;
}

export interface WorkPayloadResolution {
  status: WorkPayloadResolutionStatus;
  reason?: string;
  snapshot?: WorkPayloadSnapshot;
}

export interface WorkPayloadLineageProjection {
  snapshots_by_id: Record<string, WorkPayloadSnapshot>;
  initial_snapshot_id_by_work_id: Record<string, string>;
  latest_snapshot_id_by_work_id: Record<string, string>;
  consumed_snapshot_refs_by_dispatch_id: Record<
    string,
    Record<string, WorkPayloadRef>
  >;
  output_snapshot_refs_by_dispatch_id: Record<
    string,
    Record<string, WorkPayloadRef>
  >;
  snapshot_ids_by_work_id: Record<string, string[]>;
}

export function emptyWorkPayloadLineageProjection(): WorkPayloadLineageProjection {
  return {
    snapshots_by_id: {},
    initial_snapshot_id_by_work_id: {},
    latest_snapshot_id_by_work_id: {},
    consumed_snapshot_refs_by_dispatch_id: {},
    output_snapshot_refs_by_dispatch_id: {},
    snapshot_ids_by_work_id: {},
  };
}

export function recordWorkRequestSnapshot(
  projection: WorkPayloadLineageProjection,
  observedTick: number,
  requestID: string,
  item: FactoryWorkItem,
): void {
  if (item.id === "") {
    return;
  }

  let logicalWorkID = item.id;
  let continuity: WorkPayloadContinuity = "INITIAL_SUBMISSION";
  let parentSnapshotIDs: string[] | undefined;
  let parentWorkIDs: string[] | undefined;
  let parentLogicalWorkIDs: string[] | undefined;

  const latest = snapshotByID(
    projection,
    projection.latest_snapshot_id_by_work_id[item.id],
  );
  if (latest) {
    logicalWorkID = latest.logical_work_id;
    continuity = "SAME_WORK_ID_CONTINUATION";
    parentSnapshotIDs = [latest.snapshot_id];
    parentWorkIDs = [latest.work_id];
    parentLogicalWorkIDs = [latest.logical_work_id];
  }

  const snapshot: WorkPayloadSnapshot = {
    snapshot_id: `work-request:${requestID}:${item.id}:${(projection.snapshot_ids_by_work_id[item.id]?.length ?? 0) + 1}`,
    work_id: item.id,
    logical_work_id: logicalWorkID,
    source_kind: "WORK_REQUEST",
    source_event_type: "WORK_REQUEST",
    request_id: requestID,
    observed_tick: observedTick,
    continuity,
    parent_snapshot_ids: parentSnapshotIDs,
    parent_work_ids: parentWorkIDs,
    parent_logical_work_ids: parentLogicalWorkIDs,
    work_item: cloneLineageWorkItem(item),
  };

  projection.snapshots_by_id[snapshot.snapshot_id] = snapshot;
  if (!(item.id in projection.initial_snapshot_id_by_work_id)) {
    projection.initial_snapshot_id_by_work_id[item.id] = snapshot.snapshot_id;
  }
  projection.latest_snapshot_id_by_work_id[item.id] = snapshot.snapshot_id;
  const snapshotIDs = projection.snapshot_ids_by_work_id[item.id] ?? [];
  projection.snapshot_ids_by_work_id[item.id] = [
    ...snapshotIDs,
    snapshot.snapshot_id,
  ];
}

export function recordConsumedInputSnapshot(
  projection: WorkPayloadLineageProjection,
  dispatchID: string,
  item: FactoryWorkItem,
): void {
  if (dispatchID === "" || item.id === "") {
    return;
  }

  const dispatchRefs =
    projection.consumed_snapshot_refs_by_dispatch_id[dispatchID] ?? {};
  projection.consumed_snapshot_refs_by_dispatch_id[dispatchID] = dispatchRefs;

  const latest = snapshotByID(
    projection,
    projection.latest_snapshot_id_by_work_id[item.id],
  );
  if (latest) {
    dispatchRefs[item.id] = {
      status: "RESOLVED",
      snapshot_id: latest.snapshot_id,
    };
    return;
  }

  dispatchRefs[item.id] = {
    status: "UNAVAILABLE",
    reason:
      "no lineage snapshot was recorded before this dispatch consumed the work item",
  };
}

export function recordDispatchOutputSnapshot(
  projection: WorkPayloadLineageProjection,
  observedTick: number,
  dispatchID: string,
  consumedInputs: FactoryWorkItem[],
  item: FactoryWorkItem,
  outputIndex: number,
): void {
  if (dispatchID === "" || item.id === "") {
    return;
  }

  const parentSnapshots = resolvedConsumedSnapshotsForDispatch(
    projection,
    dispatchID,
  );
  const parentSnapshotIDs: string[] = [];
  const parentWorkIDs: string[] = [];
  const parentLogicalWorkIDs: string[] = [];
  for (const snapshot of parentSnapshots) {
    appendUniqueString(parentSnapshotIDs, snapshot.snapshot_id);
    appendUniqueString(parentWorkIDs, snapshot.work_id);
    appendUniqueString(parentLogicalWorkIDs, snapshot.logical_work_id);
  }

  let logicalWorkID = item.id;
  let continuity: WorkPayloadContinuity = "NEW_DOWNSTREAM_WORK";
  for (const snapshot of parentSnapshots) {
    if (snapshot.work_id === item.id) {
      logicalWorkID = snapshot.logical_work_id;
      continuity = "SAME_WORK_ID_CONTINUATION";
      break;
    }
  }
  if (continuity === "NEW_DOWNSTREAM_WORK" && parentSnapshots.length === 0) {
    for (const input of consumedInputs) {
      if (input.id === item.id) {
        logicalWorkID = item.id;
        continuity = "SAME_WORK_ID_CONTINUATION";
        break;
      }
    }
  }

  const snapshot: WorkPayloadSnapshot = {
    snapshot_id: `dispatch-output:${dispatchID}:${item.id}:${outputIndex}`,
    work_id: item.id,
    logical_work_id: logicalWorkID,
    source_kind: "DISPATCH_RESPONSE_OUTPUT",
    source_event_type: "DISPATCH_RESPONSE_OUTPUT",
    dispatch_id: dispatchID,
    observed_tick: observedTick,
    continuity,
    parent_snapshot_ids:
      parentSnapshotIDs.length > 0 ? parentSnapshotIDs : undefined,
    parent_work_ids: parentWorkIDs.length > 0 ? parentWorkIDs : undefined,
    parent_logical_work_ids:
      parentLogicalWorkIDs.length > 0 ? parentLogicalWorkIDs : undefined,
    work_item: cloneLineageWorkItem(item),
  };

  projection.snapshots_by_id[snapshot.snapshot_id] = snapshot;
  projection.latest_snapshot_id_by_work_id[item.id] = snapshot.snapshot_id;
  const snapshotIDs = projection.snapshot_ids_by_work_id[item.id] ?? [];
  projection.snapshot_ids_by_work_id[item.id] = [
    ...snapshotIDs,
    snapshot.snapshot_id,
  ];

  const dispatchRefs =
    projection.output_snapshot_refs_by_dispatch_id[dispatchID] ?? {};
  projection.output_snapshot_refs_by_dispatch_id[dispatchID] = dispatchRefs;
  dispatchRefs[item.id] = {
    status: "RESOLVED",
    snapshot_id: snapshot.snapshot_id,
  };
}

export function resolveInitialSubmittedSnapshot(
  projection: WorkPayloadLineageProjection,
  workID: string,
): WorkPayloadResolution {
  return resolveSnapshotID(
    projection,
    projection.initial_snapshot_id_by_work_id[workID],
    "no initial work-request payload snapshot was recorded for this work item",
  );
}

export function resolveConsumedInputSnapshot(
  projection: WorkPayloadLineageProjection,
  dispatchID: string,
  workID: string,
): WorkPayloadResolution {
  const dispatchRefs =
    projection.consumed_snapshot_refs_by_dispatch_id[dispatchID];
  if (!dispatchRefs) {
    return unavailableWorkPayloadResolution(
      "no consumed-input lineage was recorded for this dispatch",
    );
  }
  const ref = dispatchRefs[workID];
  if (!ref) {
    return unavailableWorkPayloadResolution(
      "the dispatch did not record a consumed lineage snapshot for this work item",
    );
  }
  return resolveRef(projection, ref);
}

export function resolveSelectedWorkSnapshot(
  projection: WorkPayloadLineageProjection,
  workID: string,
): WorkPayloadResolution {
  return resolveSnapshotID(
    projection,
    projection.latest_snapshot_id_by_work_id[workID],
    "no lineage snapshot is available for this work item",
  );
}

export function resolveOutputWorkSnapshot(
  projection: WorkPayloadLineageProjection,
  dispatchID: string,
  workID: string,
): WorkPayloadResolution {
  const dispatchRefs =
    projection.output_snapshot_refs_by_dispatch_id[dispatchID];
  if (!dispatchRefs) {
    return unavailableWorkPayloadResolution(
      "no output-work lineage was recorded for this dispatch",
    );
  }
  const ref = dispatchRefs[workID];
  if (!ref) {
    return unavailableWorkPayloadResolution(
      "the dispatch did not record an output lineage snapshot for this work item",
    );
  }
  return resolveRef(projection, ref);
}

function resolveRef(
  projection: WorkPayloadLineageProjection,
  ref: WorkPayloadRef,
): WorkPayloadResolution {
  if (ref.status === "UNAVAILABLE") {
    return unavailableWorkPayloadResolution(ref.reason ?? "");
  }
  return resolveSnapshotID(projection, ref.snapshot_id, ref.reason ?? "");
}

function resolveSnapshotID(
  projection: WorkPayloadLineageProjection,
  snapshotID: string | undefined,
  unavailableReason: string,
): WorkPayloadResolution {
  const snapshot = snapshotByID(projection, snapshotID);
  if (!snapshot) {
    return unavailableWorkPayloadResolution(unavailableReason);
  }
  return {
    status: "RESOLVED",
    snapshot: cloneLineageSnapshot(snapshot),
  };
}

function unavailableWorkPayloadResolution(
  reason: string,
): WorkPayloadResolution {
  return {
    status: "UNAVAILABLE",
    reason,
  };
}

function snapshotByID(
  projection: WorkPayloadLineageProjection,
  snapshotID: string | undefined,
): WorkPayloadSnapshot | undefined {
  if (!snapshotID) {
    return undefined;
  }
  return projection.snapshots_by_id[snapshotID];
}

function resolvedConsumedSnapshotsForDispatch(
  projection: WorkPayloadLineageProjection,
  dispatchID: string,
): WorkPayloadSnapshot[] {
  const dispatchRefs =
    projection.consumed_snapshot_refs_by_dispatch_id[dispatchID];
  if (!dispatchRefs || Object.keys(dispatchRefs).length === 0) {
    return [];
  }

  const snapshots: WorkPayloadSnapshot[] = [];
  for (const workID of sortedStringKeys(dispatchRefs)) {
    const ref = dispatchRefs[workID];
    if (ref.status !== "RESOLVED") {
      continue;
    }
    const snapshot = snapshotByID(projection, ref.snapshot_id);
    if (!snapshot) {
      continue;
    }
    snapshots.push(snapshot);
  }
  return snapshots;
}

function cloneLineageSnapshot(
  snapshot: WorkPayloadSnapshot,
): WorkPayloadSnapshot {
  return {
    ...snapshot,
    parent_snapshot_ids: cloneStringSlice(snapshot.parent_snapshot_ids),
    parent_work_ids: cloneStringSlice(snapshot.parent_work_ids),
    parent_logical_work_ids: cloneStringSlice(snapshot.parent_logical_work_ids),
    work_item: cloneLineageWorkItem(snapshot.work_item),
  };
}

export function cloneLineageWorkItem(item: FactoryWorkItem): FactoryWorkItem {
  return {
    ...item,
    previous_chaining_trace_ids: cloneStringSlice(
      item.previous_chaining_trace_ids,
    ),
    content: cloneWorkContent(item.content),
    tags: item.tags ? { ...item.tags } : undefined,
  };
}

function cloneWorkContent(
  content: WorkContent | undefined,
): WorkContent | undefined {
  if (!content) {
    return undefined;
  }
  return content.map((part) => ({ ...part }));
}

function cloneStringSlice(values: string[] | undefined): string[] | undefined {
  if (!values) {
    return undefined;
  }
  return [...values];
}

function sortedStringKeys(values: Record<string, WorkPayloadRef>): string[] {
  return Object.keys(values).sort();
}

function appendUniqueString(values: string[], value: string): void {
  if (value === "" || values.includes(value)) {
    return;
  }
  values.push(value);
}

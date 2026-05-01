import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import type { DashboardTrace, DashboardTraceDispatch, DashboardWorkRelation } from "../../api/dashboard/types";
import { useFactoryTimelineStore } from "../../state/factoryTimelineStore";

const DASHBOARD_WORK_TRACE_QUERY_KEY = ["agent-factory-work-trace"] as const;

export function expandTraceWithCausalPredecessors(
  trace: DashboardTrace | undefined,
  tracesByWorkID: Record<string, DashboardTrace>,
): DashboardTrace | undefined {
  if (!trace?.trace_id) {
    return trace;
  }

  const tracesByID = indexTracesByID(tracesByWorkID);
  if (Object.keys(tracesByID).length === 0) {
    return trace;
  }

  const includedTraceIDs = new Set<string>();
  const pendingTraceIDs = [trace.trace_id];

  while (pendingTraceIDs.length > 0) {
    const traceID = pendingTraceIDs.shift();
    if (!traceID || includedTraceIDs.has(traceID)) {
      continue;
    }

    const currentTrace = tracesByID[traceID];
    if (!currentTrace) {
      continue;
    }

    includedTraceIDs.add(traceID);

    for (const predecessorTraceID of predecessorTraceIDsForDispatches(currentTrace.dispatches)) {
      if (!includedTraceIDs.has(predecessorTraceID) && tracesByID[predecessorTraceID]) {
        pendingTraceIDs.push(predecessorTraceID);
      }
    }
  }

  if (includedTraceIDs.size <= 1) {
    return trace;
  }

  const includedTraces = [...includedTraceIDs]
    .map((traceID) => tracesByID[traceID])
    .filter((candidate): candidate is DashboardTrace => candidate !== undefined);
  const dispatches = sortDispatchesByTime(
    dedupeDispatches(
      includedTraces.flatMap((candidate) => candidate.dispatches),
    ),
  );
  const requestIDs = sortedUniqueNonEmptyStrings(
    includedTraces.flatMap((candidate) => candidate.request_ids ?? []),
  );
  const transitionIDs = sortedUniqueNonEmptyStrings(
    dispatches.map((dispatch) => dispatch.transition_id),
  );
  const workIDs = sortedUniqueNonEmptyStrings(
    includedTraces.flatMap((candidate) => candidate.work_ids),
  );
  const relations = dedupeRelations(
    includedTraces.flatMap((candidate) => candidate.relations ?? []),
  );

  return {
    ...trace,
    dispatches,
    relations: relations.length > 0 ? relations : trace.relations,
    request_ids: requestIDs.length > 0 ? requestIDs : trace.request_ids,
    transition_ids: transitionIDs,
    work_ids: workIDs,
    work_items: undefined,
    workstation_sequence: dispatches.map(
      (dispatch) => dispatch.workstation_name || dispatch.transition_id,
    ),
  };
}

export function useDashboardTrace(workID: string | null, traceID?: string | null) {
  const selectedTick = useFactoryTimelineStore((state) => state.selectedTick);
  const tracesByWorkID = useFactoryTimelineStore(
    (state) => state.worldViewCache[state.selectedTick]?.tracesByWorkID ?? {},
  );
  const eventTrace = useFactoryTimelineStore(
    (state) => state.worldViewCache[state.selectedTick]?.tracesByWorkID[workID ?? ""],
  );
  const directTrace = useFactoryTimelineStore(
    (state) => {
      if (!traceID) {
        return undefined;
      }

      return Object.values(state.worldViewCache[state.selectedTick]?.tracesByWorkID ?? {}).find(
        (trace) => trace.trace_id === traceID,
      );
    },
  );
  const trace = useMemo(
    () => expandTraceWithCausalPredecessors(directTrace ?? eventTrace, tracesByWorkID),
    [directTrace, eventTrace, tracesByWorkID],
  );

  return useQuery({
    queryKey: [...DASHBOARD_WORK_TRACE_QUERY_KEY, workID, traceID ?? "", selectedTick],
    queryFn: () => trace,
    enabled: trace !== undefined,
    initialData: trace,
    refetchOnWindowFocus: false,
    retry: false,
  });
}

function indexTracesByID(tracesByWorkID: Record<string, DashboardTrace>): Record<string, DashboardTrace> {
  return Object.values(tracesByWorkID).reduce<Record<string, DashboardTrace>>((indexed, trace) => {
    if (trace.trace_id && indexed[trace.trace_id] === undefined) {
      indexed[trace.trace_id] = trace;
    }
    return indexed;
  }, {});
}

function predecessorTraceIDsForDispatches(dispatches: DashboardTraceDispatch[]): string[] {
  return uniqueNonEmptyStrings(
    dispatches.flatMap((dispatch) => [
      ...(dispatch.previous_chaining_trace_ids ?? []),
      ...(dispatch.input_items ?? []).flatMap(
        (item) => item.previous_chaining_trace_ids ?? [],
      ),
    ]),
  );
}

function dedupeDispatches(dispatches: DashboardTraceDispatch[]): DashboardTraceDispatch[] {
  return [...new Map(
    dispatches.map((dispatch) => [dispatch.dispatch_id, dispatch] as const),
  ).values()];
}

function sortDispatchesByTime(dispatches: DashboardTraceDispatch[]): DashboardTraceDispatch[] {
  return [...dispatches].sort((left, right) => {
    if (left.start_time !== right.start_time) {
      return left.start_time.localeCompare(right.start_time);
    }
    if (left.end_time !== right.end_time) {
      return left.end_time.localeCompare(right.end_time);
    }
    return left.dispatch_id.localeCompare(right.dispatch_id);
  });
}

function dedupeRelations(relations: DashboardWorkRelation[]): DashboardWorkRelation[] {
  const keyedRelations = new Map<string, DashboardWorkRelation>();

  for (const relation of relations) {
    const key = [
      relation.type,
      relation.source_work_id ?? "",
      relation.target_work_id,
      relation.required_state ?? "",
      relation.request_id ?? "",
    ].join("|");
    keyedRelations.set(key, relation);
  }

  return [...keyedRelations.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([, relation]) => relation);
}

function uniqueNonEmptyStrings(values: Array<string | undefined>): string[] {
  return [...new Set(values.filter((value): value is string => Boolean(value && value.trim())))];
}

function sortedUniqueNonEmptyStrings(values: Array<string | undefined>): string[] {
  return uniqueNonEmptyStrings(values).sort((left, right) => left.localeCompare(right));
}

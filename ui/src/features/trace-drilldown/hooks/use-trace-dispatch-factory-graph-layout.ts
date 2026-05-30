import type { FactoryGraphTopology } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import {
  createWorkflowTopologyAsyncCache,
  useWorkflowTopologyAsyncCache,
} from "../../workflow-activity/hooks/workflow-topology-async-cache";
import {
  buildTraceFactoryGraphLayoutPositions,
  type TraceFactoryGraphLayoutPosition,
} from "../lib/trace-factory-graph-layout";

const EMPTY_POSITIONS = new Map<string, TraceFactoryGraphLayoutPosition>();
const TRACE_DISPATCH_LAYOUT_CACHE = createWorkflowTopologyAsyncCache<
  Map<string, TraceFactoryGraphLayoutPosition>
>();

export function resetTraceDispatchFactoryGraphLayoutCacheForTests() {
  TRACE_DISPATCH_LAYOUT_CACHE.resolvedByTopologyKey.clear();
  TRACE_DISPATCH_LAYOUT_CACHE.inFlightByTopologyKey.clear();
}

export function traceDispatchTopologyLayoutKey(
  topology: FactoryGraphTopology,
): string {
  return JSON.stringify({
    edges: topology.edges
      .map((edge) => `${edge.id}:${edge.source}:${edge.target}`)
      .sort(),
    nodes: topology.nodes.map((node) => node.id).sort(),
  });
}

export function useTraceDispatchFactoryGraphLayoutPositions(
  topology: FactoryGraphTopology,
  dispatchIdByNodeId: ReadonlyMap<string, string>,
  topologyKey: string,
) {
  return useWorkflowTopologyAsyncCache({
    cache: TRACE_DISPATCH_LAYOUT_CACHE,
    dependencies: [topology, dispatchIdByNodeId],
    disabled: topology.nodes.length === 0,
    disabledValue: EMPTY_POSITIONS,
    fallbackValue: EMPTY_POSITIONS,
    initialValue: EMPTY_POSITIONS,
    loadLayout: () =>
      buildTraceFactoryGraphLayoutPositions(topology, dispatchIdByNodeId),
    mapResolvedLayout: (positions) => positions,
    topologyKey,
  });
}

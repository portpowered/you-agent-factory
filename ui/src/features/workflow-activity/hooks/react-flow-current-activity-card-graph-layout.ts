import { useMemo } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { buildGraphLayout, type GraphLayout } from "../../flowchart/lib/layout";
import { EMPTY_GRAPH_LAYOUT } from "../lib/react-flow-current-activity-card-graph";
import { currentActivityTopologyKey } from "../lib/react-flow-current-activity-card-keys";
import {
  createWorkflowTopologyAsyncCache,
  useWorkflowTopologyAsyncCache,
} from "./workflow-topology-async-cache";

const GRAPH_LAYOUT_CACHE = createWorkflowTopologyAsyncCache<GraphLayout>();

export function useCurrentActivityGraphLayout(snapshot: DashboardSnapshot) {
  const topologyKey = useMemo(
    () => currentActivityTopologyKey(snapshot.topology),
    [snapshot.topology],
  );
  const layoutTopology = useMemo(() => snapshot.topology, [snapshot.topology]);

  return useWorkflowTopologyAsyncCache({
    cache: GRAPH_LAYOUT_CACHE,
    dependencies: [layoutTopology],
    fallbackValue: EMPTY_GRAPH_LAYOUT,
    initialValue: EMPTY_GRAPH_LAYOUT,
    loadLayout: () => buildGraphLayout(layoutTopology),
    mapResolvedLayout: identityGraphLayout,
    topologyKey,
  });
}

function identityGraphLayout(layout: GraphLayout) {
  return layout;
}

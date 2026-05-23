import { buildFactoryGraphEditorLayout } from "../../factory-graph-editor/lib/factory-graph-editor-layout";
import type { FactoryGraphTopology } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import {
  createWorkflowTopologyAsyncCache,
  useWorkflowTopologyAsyncCache,
} from "./workflow-topology-async-cache";

const EMPTY_LAYOUT_POSITIONS = new Map<string, { x: number; y: number }>();
const EDITOR_LAYOUT_CACHE = createWorkflowTopologyAsyncCache<
  Awaited<ReturnType<typeof buildFactoryGraphEditorLayout>>
>();

export function useFactoryGraphEditorLayoutPositions(
  topology: FactoryGraphTopology,
  topologyKey: string,
) {
  return useWorkflowTopologyAsyncCache({
    cache: EDITOR_LAYOUT_CACHE,
    dependencies: [topology],
    disabled: topology.nodes.length === 0,
    disabledValue: EMPTY_LAYOUT_POSITIONS,
    fallbackValue: EMPTY_LAYOUT_POSITIONS,
    initialValue: EMPTY_LAYOUT_POSITIONS,
    loadLayout: () => buildFactoryGraphEditorLayout(topology),
    mapResolvedLayout: mapLayoutNodePositions,
    topologyKey,
  });
}

function layoutNodePositions(
  nodes: Array<{ nodeId: string; x: number; y: number }>,
) {
  return new Map(nodes.map((node) => [node.nodeId, { x: node.x, y: node.y }]));
}

function mapLayoutNodePositions(
  layout: Awaited<ReturnType<typeof buildFactoryGraphEditorLayout>>,
) {
  return layoutNodePositions(layout.nodes);
}

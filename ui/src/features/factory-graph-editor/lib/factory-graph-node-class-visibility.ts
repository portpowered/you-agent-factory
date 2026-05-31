import { filterFactoryGraphTopologyForCustomerDisplay } from "./factory-graph-customer-display";
import type {
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "./factory-graph-draft-types";

export const FACTORY_GRAPH_TOGGLEABLE_NODE_KINDS = [
  "work-type",
  "work-state",
  "workstation",
  "worker",
  "resource",
] as const satisfies readonly FactoryGraphNodeKind[];

export type FactoryGraphNodeClassVisibilityInput =
  | FactoryGraphTopology
  | {
      layoutPositionsByNodeId?: ReadonlyMap<string, { x: number; y: number }>;
      topology: FactoryGraphTopology;
    };

export type FactoryGraphNodeClassVisibilityResult = FactoryGraphTopology & {
  layoutPositionsByNodeId?: Map<string, { x: number; y: number }>;
};

function resolveVisibilityInput(input: FactoryGraphNodeClassVisibilityInput): {
  layoutPositionsByNodeId?: ReadonlyMap<string, { x: number; y: number }>;
  topology: FactoryGraphTopology;
} {
  if ("topology" in input) {
    return {
      layoutPositionsByNodeId: input.layoutPositionsByNodeId,
      topology: input.topology,
    };
  }

  return { topology: input };
}

export function projectFactoryGraphByHiddenNodeClasses(
  input: FactoryGraphNodeClassVisibilityInput,
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>,
): FactoryGraphNodeClassVisibilityResult {
  const { layoutPositionsByNodeId, topology } = resolveVisibilityInput(input);
  const displayTopology =
    filterFactoryGraphTopologyForCustomerDisplay(topology);

  const hiddenNodeIds = new Set(
    displayTopology.nodes
      .filter((node) => hiddenNodeClasses.has(node.kind))
      .map((node) => node.id),
  );

  const nodes = displayTopology.nodes.filter(
    (node) => !hiddenNodeIds.has(node.id),
  );
  const visibleNodeIds = new Set(nodes.map((node) => node.id));
  const edges = displayTopology.edges.filter(
    (edge) =>
      visibleNodeIds.has(edge.sourceId) && visibleNodeIds.has(edge.targetId),
  );

  const result: FactoryGraphNodeClassVisibilityResult = { edges, nodes };

  if (layoutPositionsByNodeId) {
    result.layoutPositionsByNodeId = new Map(
      [...layoutPositionsByNodeId].filter(([nodeId]) =>
        visibleNodeIds.has(nodeId),
      ),
    );
  }

  return result;
}

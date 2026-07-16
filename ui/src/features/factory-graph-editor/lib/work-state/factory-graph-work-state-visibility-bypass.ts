import type {
  FactoryGraphEdge,
  FactoryGraphEdgeKind,
  FactoryGraphNode,
  FactoryGraphNodeReference,
  FactoryGraphTopology,
  WorkstationToWorkStateRouteKind,
} from "../draft/factory-graph-draft-types";
import { buildEdge } from "../draft/factory-graph-draft-types";

const WORKSTATION_OUTPUT_ROUTE_KINDS = [
  "workstation-output",
  "workstation-on-continue",
  "workstation-on-failure",
  "workstation-on-rejection",
] as const satisfies readonly WorkstationToWorkStateRouteKind[];

export type FactoryGraphVisibilityBypassEdge = FactoryGraphEdge & {
  kind: "work-state-visibility-bypass";
  outcomeRouteKind: WorkstationToWorkStateRouteKind;
};

export function synthesizeWorkStateVisibilityBypassEdges(
  topology: FactoryGraphTopology,
  hiddenWorkStateNodeIds: ReadonlySet<string>,
  visibleNodeIds: ReadonlySet<string>,
): FactoryGraphVisibilityBypassEdge[] {
  if (hiddenWorkStateNodeIds.size === 0) {
    return [];
  }

  const nodesById = new Map(topology.nodes.map((node) => [node.id, node]));
  const bypassEdges: FactoryGraphVisibilityBypassEdge[] = [];
  const seenBypassIds = new Set<string>();

  for (const workStateId of hiddenWorkStateNodeIds) {
    const producers = collectWorkstationProducersForState(
      topology.edges,
      workStateId,
    );
    const consumers = collectWorkstationConsumersForState(
      topology.edges,
      workStateId,
    );

    for (const producer of producers) {
      for (const consumer of consumers) {
        if (
          !visibleNodeIds.has(producer.workstationId) ||
          !visibleNodeIds.has(consumer.workstationId)
        ) {
          continue;
        }

        const bypass = buildWorkStateVisibilityBypassEdge({
          consumer,
          nodesById,
          producer,
          workStateId,
        });
        if (seenBypassIds.has(bypass.id)) {
          continue;
        }
        seenBypassIds.add(bypass.id);
        bypassEdges.push(bypass);
      }
    }
  }

  return bypassEdges.sort((left, right) => left.id.localeCompare(right.id));
}

function collectWorkstationProducersForState(
  edges: readonly FactoryGraphEdge[],
  workStateId: string,
) {
  const producers: Array<{
    outcomeRouteKind: WorkstationToWorkStateRouteKind;
    workstationId: string;
  }> = [];

  for (const edge of edges) {
    if (edge.targetId !== workStateId) {
      continue;
    }
    if (!isWorkstationToWorkStateRouteKind(edge.kind)) {
      continue;
    }
    producers.push({
      outcomeRouteKind: edge.kind,
      workstationId: edge.sourceId,
    });
  }

  return producers;
}

function collectWorkstationConsumersForState(
  edges: readonly FactoryGraphEdge[],
  workStateId: string,
) {
  const consumers: Array<{ workstationId: string }> = [];

  for (const edge of edges) {
    if (edge.sourceId !== workStateId || edge.kind !== "workstation-input") {
      continue;
    }
    consumers.push({ workstationId: edge.targetId });
  }

  return consumers;
}

function buildWorkStateVisibilityBypassEdge(input: {
  consumer: { workstationId: string };
  nodesById: ReadonlyMap<string, FactoryGraphNode>;
  producer: {
    outcomeRouteKind: WorkstationToWorkStateRouteKind;
    workstationId: string;
  };
  workStateId: string;
}): FactoryGraphVisibilityBypassEdge {
  const source = workstationReference(
    input.nodesById,
    input.producer.workstationId,
  );
  const target = workstationReference(
    input.nodesById,
    input.consumer.workstationId,
  );
  const edge = buildEdge(
    "work-state-visibility-bypass",
    source,
    target,
  ) as FactoryGraphVisibilityBypassEdge;

  return {
    ...edge,
    id: [
      "work-state-visibility-bypass",
      input.producer.outcomeRouteKind,
      `${source.name}->${target.name}`,
      `via-${input.workStateId}`,
    ].join(":"),
    outcomeRouteKind: input.producer.outcomeRouteKind,
  };
}

function workstationReference(
  nodesById: ReadonlyMap<string, FactoryGraphNode>,
  workstationId: string,
): FactoryGraphNodeReference {
  const node = nodesById.get(workstationId);
  if (node?.key.kind !== "workstation") {
    throw new Error(`Expected workstation node ${workstationId}`);
  }

  return node.key;
}

function isWorkstationToWorkStateRouteKind(
  kind: FactoryGraphEdgeKind,
): kind is WorkstationToWorkStateRouteKind {
  return (WORKSTATION_OUTPUT_ROUTE_KINDS as readonly string[]).includes(kind);
}

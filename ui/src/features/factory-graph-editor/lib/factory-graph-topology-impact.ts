import { buildFactoryGraphTopologyFromDefinition } from "./factory-graph-draft-graph";
import type { CanonicalFactoryDefinition } from "./factory-graph-draft-types";

/**
 * Whether replacing `previous` with `next` changes nodes or edges on the factory graph.
 * Uses the same topology projection as graph rendering so non-structural field edits
 * (prompt, cron, model, capacity, handling behavior, and similar) do not report impact.
 */
export function doesFactoryDefinitionChangeAffectGraphTopology(
  previous: CanonicalFactoryDefinition,
  next: CanonicalFactoryDefinition,
): boolean {
  return (
    factoryGraphTopologySignature(previous) !==
    factoryGraphTopologySignature(next)
  );
}

function factoryGraphTopologySignature(
  definition: CanonicalFactoryDefinition,
): string {
  const topology = buildFactoryGraphTopologyFromDefinition(definition);
  return JSON.stringify({
    edgeIds: topology.edges.map((edge) => edge.id).sort(),
    nodeIds: topology.nodes.map((node) => node.id).sort(),
  });
}

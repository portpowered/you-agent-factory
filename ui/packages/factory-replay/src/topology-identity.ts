import type { FactoryTopologyNodeKind } from "./topology-contract.js";

/** Resolve the durable public identity used by every topology projection. */
export function factoryTopologyEntityId(
  explicitId: string | undefined,
  name: string,
): string {
  return explicitId?.trim() || name;
}

/** Build the renderer-facing node identity for one canonical Factory entity. */
export function factoryTopologyNodeId(
  kind: FactoryTopologyNodeKind,
  entityId: string,
): string {
  return `${kind}:${entityId}`;
}

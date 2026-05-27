import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import {
  type FactoryGraphNodeKey,
  type FactoryGraphNodeKind,
  nodeKeyId,
} from "../../factory-graph-editor/lib/factory-graph-draft-types";

export function resolveFactoryGraphPlaceNode(
  place: DashboardPlaceRef,
): { kind: FactoryGraphNodeKind; nodeId: string } | null {
  if (
    place.kind === "work_state" &&
    typeof place.type_id === "string" &&
    typeof place.state_value === "string"
  ) {
    return {
      kind: "work-state",
      nodeId: nodeKeyId({
        kind: "work-state",
        stateName: place.state_value,
        workTypeName: place.type_id,
      }),
    };
  }
  if (place.kind === "resource" && typeof place.type_id === "string") {
    return {
      kind: "resource",
      nodeId: nodeKeyId({ kind: "resource", name: place.type_id }),
    };
  }
  if (
    place.kind === "constraint" &&
    place.type_id === "worker" &&
    typeof place.state_value === "string"
  ) {
    return {
      kind: "worker",
      nodeId: nodeKeyId({ kind: "worker", name: place.state_value }),
    };
  }
  if (
    place.kind === "constraint" &&
    place.type_id === "work-type" &&
    typeof place.state_value === "string"
  ) {
    return {
      kind: "work-type",
      nodeId: nodeKeyId({ kind: "work-type", name: place.state_value }),
    };
  }

  return null;
}

export function currentActivityNodeIdForFactoryGraphKey(
  key: FactoryGraphNodeKey,
): string {
  if (key.kind === "workstation") {
    return `workstation:${key.name}`;
  }
  if (key.kind === "resource") {
    return `place:${key.name}:available`;
  }
  if (key.kind === "worker" || key.kind === "work-type") {
    return `place:${key.kind}:${key.name}`;
  }
  if (key.kind === "work-state") {
    return `place:${key.workTypeName}:${key.stateName}`;
  }

  return `place:${key.name}`;
}

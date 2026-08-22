import type { components, FactoryDefinition } from "@you-agent-factory/client";
import {
  WorkstationGuardType,
  WorkstationKind,
  WorkstationType,
} from "@you-agent-factory/client";
import type { FactoryTopologyNode } from "@you-agent-factory/factory-replay";

import type { FactoryGraphSource } from "./source.js";

type FactoryWorkstation = NonNullable<
  FactoryDefinition["workstations"]
>[number];
type FactoryWorkstationGuard = NonNullable<
  FactoryWorkstation["guards"]
>[number];

export type FactoryGraphWorkstationRuntimeType =
  | components["schemas"]["WorkstationType"]
  | "UNKNOWN";

export type FactoryGraphWorkstationRuntimeRole =
  | "AGENT"
  | "CLASSIFIER"
  | "HUMAN_APPROVAL"
  | "INFERENCE"
  | "LOGICAL_MOVE"
  | "POLLER"
  | "SCRIPT"
  | "UNKNOWN";

export type FactoryGraphWorkstationSchedulingBehavior =
  | components["schemas"]["WorkstationKind"]
  | "UNKNOWN";

export type FactoryGraphWorkstationControlRole =
  | "CLASSIFIER"
  | "HUMAN_APPROVAL"
  | "LOGICAL_ROUTER"
  | "LOOP_BREAKER"
  | "NONE"
  | "UNKNOWN";

export interface FactoryGraphWorkstationGuardedControl {
  guardType: "VISIT_COUNT";
  limit: {
    argument?: string;
    fixed?: number;
  };
  targetWorkstation: string;
}

export interface FactoryGraphWorkstationSemantics {
  controlRole: FactoryGraphWorkstationControlRole;
  guardedControl?: FactoryGraphWorkstationGuardedControl;
  runtimeRole: FactoryGraphWorkstationRuntimeRole;
  runtimeType: FactoryGraphWorkstationRuntimeType;
  schedulingBehavior: FactoryGraphWorkstationSchedulingBehavior;
}

export interface FactoryGraphWorkstationActivityProjection {
  active: boolean;
  activeDispatchIds: string[];
}

export interface FactoryGraphWorkstationSemanticProjection
  extends FactoryGraphWorkstationSemantics {
  activity: FactoryGraphWorkstationActivityProjection;
  authoredWorkstationId?: string;
  nodeId: string;
  workstationId: string;
  workstationName: string;
}

export const UNKNOWN_FACTORY_GRAPH_WORKSTATION_SEMANTICS = {
  controlRole: "UNKNOWN",
  runtimeRole: "UNKNOWN",
  runtimeType: "UNKNOWN",
  schedulingBehavior: "UNKNOWN",
} as const satisfies FactoryGraphWorkstationSemantics;

const WORKSTATION_TYPE_VALUES = new Set<string>(Object.values(WorkstationType));
const WORKSTATION_KIND_VALUES = new Set<string>(Object.values(WorkstationKind));

/** Resolve the canonical runtime type without inferring it from worker data. */
export function resolveFactoryGraphWorkstationRuntimeType(
  workstation: Pick<FactoryWorkstation, "type"> | undefined,
): FactoryGraphWorkstationRuntimeType {
  const normalized = normalizeEnumValue(workstation?.type);
  return normalized && WORKSTATION_TYPE_VALUES.has(normalized)
    ? (normalized as FactoryGraphWorkstationRuntimeType)
    : "UNKNOWN";
}

/** Map canonical and legacy runtime type values onto graph-level roles. */
export function factoryGraphWorkstationRuntimeRole(
  runtimeType: FactoryGraphWorkstationRuntimeType,
): FactoryGraphWorkstationRuntimeRole {
  switch (runtimeType) {
    case WorkstationType.AGENT_RUN:
    case WorkstationType.MODEL_WORKSTATION:
      return "AGENT";
    case WorkstationType.CLASSIFIER_WORKSTATION:
      return "CLASSIFIER";
    case WorkstationType.HUMAN_APPROVAL:
      return "HUMAN_APPROVAL";
    case WorkstationType.INFERENCE_RUN:
    case WorkstationType.MODEL_INVOKE:
      return "INFERENCE";
    case WorkstationType.LOGICAL_MOVE:
      return "LOGICAL_MOVE";
    case WorkstationType.POLLER_RUN:
      return "POLLER";
    case WorkstationType.SCRIPT_RUN:
      return "SCRIPT";
    default:
      return "UNKNOWN";
  }
}

/** Resolve scheduling independently from runtime implementation type. */
export function resolveFactoryGraphWorkstationSchedulingBehavior(
  workstation: Pick<FactoryWorkstation, "behavior"> | undefined,
): FactoryGraphWorkstationSchedulingBehavior {
  if (!workstation) return "UNKNOWN";
  const normalized = normalizeEnumValue(workstation.behavior);
  return normalized && WORKSTATION_KIND_VALUES.has(normalized)
    ? (normalized as FactoryGraphWorkstationSchedulingBehavior)
    : "UNKNOWN";
}

/**
 * Project one authored workstation. Missing or invalid type metadata stays
 * neutral; worker presence, names, and topology shape never fill it in.
 */
export function resolveFactoryGraphWorkstationSemantics(
  workstation: FactoryWorkstation | undefined,
): FactoryGraphWorkstationSemantics {
  if (!workstation) return UNKNOWN_FACTORY_GRAPH_WORKSTATION_SEMANTICS;

  const runtimeType = resolveFactoryGraphWorkstationRuntimeType(workstation);
  const runtimeRole = factoryGraphWorkstationRuntimeRole(runtimeType);
  const guardedControl =
    runtimeRole === "LOGICAL_MOVE"
      ? resolveSupportedVisitCountGuard(workstation.guards)
      : undefined;
  return {
    controlRole: workstationControlRole(runtimeRole, guardedControl),
    ...(guardedControl ? { guardedControl } : {}),
    runtimeRole,
    runtimeType,
    schedulingBehavior:
      resolveFactoryGraphWorkstationSchedulingBehavior(workstation),
  };
}

/**
 * Join authored workstation semantics to selected-tick activity by durable
 * workstation identity. Names are a fallback only for id-less legacy authors.
 */
export function projectFactoryGraphWorkstationSemantics(
  source: FactoryGraphSource,
): FactoryGraphWorkstationSemanticProjection[] {
  const authored = source.factory.workstations ?? [];
  const topologyWorkstations = source.runtime.topology.nodes.filter(
    (node): node is Extract<FactoryTopologyNode, { kind: "workstation" }> =>
      node.kind === "workstation",
  );

  return topologyWorkstations.map((node) => {
    const authoredWorkstation = authoredWorkstationForNode(authored, node);
    const semantics =
      resolveFactoryGraphWorkstationSemantics(authoredWorkstation);
    const activeDispatchIds = source.runtime.activity.activeDispatchOverlays
      .filter((overlay) =>
        overlayMatchesWorkstation(overlay, node.entityId, node.id),
      )
      .map((overlay) => overlay.dispatchId)
      .sort();
    const active =
      activeDispatchIds.length > 0 ||
      source.runtime.activity.activeWorkstationNodeIds.includes(node.id);

    return {
      ...semantics,
      activity: { active, activeDispatchIds },
      ...(authoredWorkstation
        ? {
            authoredWorkstationId:
              authoredWorkstation.id?.trim() || authoredWorkstation.name,
          }
        : {}),
      nodeId: node.id,
      workstationId: node.entityId,
      workstationName: node.label,
    };
  });
}

function authoredWorkstationForNode(
  workstations: readonly FactoryWorkstation[],
  node: Extract<FactoryTopologyNode, { kind: "workstation" }>,
): FactoryWorkstation | undefined {
  return workstations.find((workstation) => {
    const authoredId = workstation.id?.trim();
    return authoredId
      ? authoredId === node.entityId
      : workstation.name === node.label;
  });
}

function overlayMatchesWorkstation(
  overlay: { workstationId?: string; workstationNodeId?: string },
  workstationId: string,
  workstationNodeId: string,
): boolean {
  return (
    overlay.workstationNodeId === workstationNodeId ||
    overlay.workstationId === workstationId
  );
}

function workstationControlRole(
  runtimeRole: FactoryGraphWorkstationRuntimeRole,
  guardedControl: FactoryGraphWorkstationGuardedControl | undefined,
): FactoryGraphWorkstationControlRole {
  if (runtimeRole === "UNKNOWN") return "UNKNOWN";
  if (runtimeRole === "CLASSIFIER") return "CLASSIFIER";
  if (runtimeRole === "HUMAN_APPROVAL") return "HUMAN_APPROVAL";
  if (runtimeRole !== "LOGICAL_MOVE") return "NONE";
  return guardedControl ? "LOOP_BREAKER" : "LOGICAL_ROUTER";
}

function resolveSupportedVisitCountGuard(
  guards: readonly FactoryWorkstationGuard[] | undefined,
): FactoryGraphWorkstationGuardedControl | undefined {
  for (const guard of guards ?? []) {
    const type = normalizeEnumValue(guard.type);
    if (type !== WorkstationGuardType.VISIT_COUNT) continue;
    const targetWorkstation = guard.workstation?.trim();
    if (!targetWorkstation) continue;
    const fixedLimit =
      typeof guard.maxVisits === "number" &&
      Number.isInteger(guard.maxVisits) &&
      guard.maxVisits > 0;
    const argument = guard.maxVisitsArgument?.trim();
    const argumentLimit = Boolean(argument);
    if (!fixedLimit && !argumentLimit) continue;

    return {
      guardType: "VISIT_COUNT",
      limit: {
        ...(fixedLimit ? { fixed: guard.maxVisits } : {}),
        ...(argument ? { argument } : {}),
      },
      targetWorkstation,
    };
  }

  return undefined;
}

function normalizeEnumValue(value: string | undefined): string | undefined {
  const normalized = value?.trim().toUpperCase();
  return normalized || undefined;
}

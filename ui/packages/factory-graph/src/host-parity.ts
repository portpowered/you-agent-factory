// biome-ignore-all lint/style/noExcessiveLinesPerFile: parity projection and assertion stay co-located so every contract field is audited together.

import {
  type FactoryGraphGroupRegionInput,
  projectFactoryGraphGroupRegions,
} from "./group-region-presentation.js";
import type { FactoryGraphNodeFamily } from "./node-family.js";
import { FACTORY_GRAPH_NODE_TYPES } from "./semantic-nodes.js";
import {
  type FactoryGraphVisualState,
  resolveFactoryGraphVisualState,
} from "./visual-state.js";
import { factoryGraphWorkProgressMode } from "./work-progress-presentation.js";
import {
  type FactoryGraphWorkstationSemantics,
  UNKNOWN_FACTORY_GRAPH_WORKSTATION_SEMANTICS,
} from "./workstation-semantics.js";

/** Canonical hosts that must retain the package-owned Factory graph contract. */
export const FACTORY_GRAPH_HOST_PARITY_HOSTS = [
  "current-activity",
  "editor",
  "replay",
  "trace",
] as const;

export type FactoryGraphHostParityHost =
  (typeof FACTORY_GRAPH_HOST_PARITY_HOSTS)[number];

export type FactoryGraphHostParityField =
  | "dimensions"
  | "family"
  | "groups"
  | "handles"
  | "semanticNodeId"
  | "type"
  | "visual"
  | "workProgress"
  | "workstationSemantics";

/**
 * Handles that are part of the shared rendered graph contract. Structural
 * work-type links and the approval output alias remain in host projections so
 * their edges keep working, but they are not interchangeable customer-facing
 * connection rails and are therefore excluded from cross-host comparisons.
 */
export const FACTORY_GRAPH_HOST_PARITY_HANDLE_CONTRACT = {
  constraint: { excluded: [], shared: [] },
  doc: { excluded: [], shared: [] },
  resource: {
    excluded: [],
    shared: ["worker-resource-source", "workstation-resource-source"],
  },
  worker: {
    excluded: [],
    shared: ["worker-assignment-source", "worker-input-target"],
  },
  "work-state": {
    excluded: ["work-type-state-target"],
    shared: ["work-state-input-target", "workstation-input-source"],
  },
  "work-type": {
    excluded: ["work-type-state-source"],
    shared: [],
  },
  workstation: {
    excluded: ["workstation-approval-source"],
    shared: [
      "worker-assignment-target",
      "workstation-input-target",
      "workstation-on-continue-source",
      "workstation-on-failure-source",
      "workstation-on-rejection-source",
      "workstation-output-source",
      "workstation-resource-target",
    ],
  },
} as const satisfies Record<
  FactoryGraphNodeFamily,
  { excluded: readonly string[]; shared: readonly string[] }
>;

export interface FactoryGraphHostParityHandle {
  id: string;
  side: "left" | "right";
  type: "source" | "target";
}

export interface FactoryGraphHostParityDimensions {
  height: number | undefined;
  initialHeight: number | undefined;
  initialWidth: number | undefined;
  measured: {
    height: number | undefined;
    width: number | undefined;
  };
  width: number | undefined;
}

export interface FactoryGraphHostParityWorkProgress {
  count: number;
  mode: ReturnType<typeof factoryGraphWorkProgressMode>;
}

export type FactoryGraphHostParityWorkstationSemantics = Pick<
  FactoryGraphWorkstationSemantics,
  "controlRole" | "runtimeRole" | "runtimeType" | "schedulingBehavior"
>;

export interface FactoryGraphHostParityNode {
  dimensions: FactoryGraphHostParityDimensions;
  family: FactoryGraphNodeFamily;
  handles: FactoryGraphHostParityHandle[];
  semanticNodeId: string;
  type: string;
  visual: FactoryGraphVisualState;
  workProgress: FactoryGraphHostParityWorkProgress;
  workstationSemantics?: FactoryGraphHostParityWorkstationSemantics;
}

export interface FactoryGraphHostParityGroup {
  bounds: {
    height: number;
    width: number;
    x: number;
    y: number;
  };
  color: string;
  id: string;
  label: string;
  nodeIds: string[];
}

export interface FactoryGraphHostParityNodeInput {
  data?: Record<string, unknown>;
  height?: number;
  id: string;
  initialHeight?: number;
  initialWidth?: number;
  measured?: {
    height?: number;
    width?: number;
  };
  selected?: boolean;
  type?: string;
  width?: number;
}

export interface FactoryGraphHostParityProjection {
  groups: FactoryGraphHostParityGroup[];
  host: string;
  nodeTypes: Readonly<Record<string, unknown>>;
  nodes: FactoryGraphHostParityNode[];
}

export interface FactoryGraphHostParityComparison {
  fields: readonly FactoryGraphHostParityField[];
  hosts: readonly string[];
  nodeIds?: readonly string[];
}

export interface AssertFactoryGraphHostParityInput {
  comparisons: readonly FactoryGraphHostParityComparison[];
  hosts: Readonly<Record<string, FactoryGraphHostParityProjection>>;
}

/**
 * Convert a host's React Flow nodes into the package-owned semantic contract.
 *
 * Interaction callbacks, editor draft badges, and trace-only labels are
 * deliberately omitted. They are overlays; the fields retained here are the
 * semantic surface that every Factory graph host must preserve.
 */
export function projectFactoryGraphHostParity(input: {
  groups?: readonly (FactoryGraphGroupRegionInput & {
    nodeIds?: readonly string[];
  })[];
  host: string;
  nodeTypes: Readonly<Record<string, unknown>>;
  nodes: readonly FactoryGraphHostParityNodeInput[];
}): FactoryGraphHostParityProjection {
  return {
    groups: projectParityGroups(input.groups),
    host: input.host,
    nodeTypes: input.nodeTypes,
    nodes: input.nodes
      .map((node) => projectParityNode(node))
      .sort((left, right) =>
        left.semanticNodeId.localeCompare(right.semanticNodeId),
      ),
  };
}

/**
 * Assert one canonical behavioral contract across selected graph hosts.
 *
 * The comparisons are explicit because trace dispatch nodes intentionally own
 * different runtime overlays and IDs. A caller can compare their shared
 * semantic fields without pretending that trace history is live activity.
 */
export function assertFactoryGraphHostParity(
  input: AssertFactoryGraphHostParityInput,
): void {
  const hostEntries = Object.entries(input.hosts);
  if (hostEntries.length === 0) {
    throw new Error("[factory-graph-parity] no host projections supplied");
  }

  for (const [host, projection] of hostEntries) {
    if (projection.host !== host) {
      throw new Error(
        `[factory-graph-parity] projection host mismatch: ${host} != ${projection.host}`,
      );
    }
    if (projection.nodeTypes !== FACTORY_GRAPH_NODE_TYPES) {
      throw new Error(
        `[factory-graph-parity] ${host} bypasses FACTORY_GRAPH_NODE_TYPES`,
      );
    }
    assertKnownNodeTypes(host, projection);
    assertRenderableDimensions(host, projection);
  }

  const firstGroups = hostEntries[0]?.[1].groups ?? [];
  for (const [host, projection] of hostEntries.slice(1)) {
    if (stableSerialize(projection.groups) !== stableSerialize(firstGroups)) {
      throw new Error(
        `[factory-graph-parity] ${host} diverges in read-only group bounds, color, label, or membership`,
      );
    }
  }

  for (const comparison of input.comparisons) {
    const projections = comparison.hosts.map((host) => {
      const projection = input.hosts[host];
      if (!projection) {
        throw new Error(
          `[factory-graph-parity] comparison references missing host ${host}`,
        );
      }
      return projection;
    });
    const nodeIds = comparison.nodeIds ?? sharedNodeIds(projections);
    for (const nodeId of nodeIds) {
      const nodes = projections.map((projection) => {
        const node = projection.nodes.find(
          (candidate) => candidate.semanticNodeId === nodeId,
        );
        if (!node) {
          throw new Error(
            `[factory-graph-parity] ${projection.host} is missing semantic node ${nodeId}`,
          );
        }
        return node;
      });
      const reference = nodes[0];
      if (!reference) continue;
      for (const field of comparison.fields) {
        const referenceValue = parityFieldValue(reference, field);
        for (let index = 1; index < nodes.length; index += 1) {
          const candidate = nodes[index];
          const candidateValue = parityFieldValue(candidate, field);
          if (
            stableSerialize(candidateValue) !== stableSerialize(referenceValue)
          ) {
            throw new Error(
              `[factory-graph-parity] ${comparison.hosts[index]} diverges from ${comparison.hosts[0]} for ${nodeId}.${field}`,
            );
          }
        }
      }
    }
  }
}

function projectParityNode(
  node: FactoryGraphHostParityNodeInput,
): FactoryGraphHostParityNode {
  const data = node.data ?? {};
  const type = node.type ?? "";
  const family = familyForNode(type, data);
  const place = recordValue(data, "place");
  const selected =
    node.selected === true ||
    booleanValue(data.selectedStateNode) ||
    booleanValue(data.selectedResource) ||
    booleanValue(data.selectedWorker) ||
    booleanValue(data.selectedWorkType) ||
    booleanValue(data.selectedWorkstation) ||
    booleanValue(data.selectedDoc);
  const active = booleanValue(data.active);
  const visual = resolveFactoryGraphVisualState({
    activeFlow: booleanValue(data.activeFlow),
    activeWork:
      family === "workstation"
        ? active
        : (numberValue(data.tokenCount) ?? 0) > 0,
    family,
    focused: booleanValue(data.focused),
    lifecycle:
      stringValue(recordValue(place, "state_category")) ??
      (active ? "PROCESSING" : undefined),
    muted: booleanValue(data.muted),
    runtimeStatus: stringValue(data.runtimeStatus),
    selected,
    validation: booleanValue(data.validationError),
  });
  const workstationSemantics = parityWorkstationSemantics(
    recordValue(data, "workstationSemantics"),
  );

  return {
    dimensions: {
      height: node.height,
      initialHeight: node.initialHeight,
      initialWidth: node.initialWidth,
      measured: {
        height: node.measured?.height,
        width: node.measured?.width,
      },
      width: node.width,
    },
    family,
    handles: parityHandles(family, data),
    semanticNodeId:
      stringValue(data.factoryGraphNodeId) ??
      stringValue(data.factoryNodeId) ??
      node.id,
    type,
    visual,
    workProgress: parityWorkProgress(family, data),
    ...(workstationSemantics
      ? { workstationSemantics }
      : family === "workstation"
        ? { workstationSemantics: UNKNOWN_FACTORY_GRAPH_WORKSTATION_SEMANTICS }
        : {}),
  };
}

function parityWorkstationSemantics(
  value: unknown,
): FactoryGraphHostParityWorkstationSemantics | undefined {
  if (!isRecord(value)) return undefined;
  const controlRole = stringValue(value.controlRole);
  const runtimeRole = stringValue(value.runtimeRole);
  const runtimeType = stringValue(value.runtimeType);
  const schedulingBehavior = stringValue(value.schedulingBehavior);
  if (!controlRole || !runtimeRole || !runtimeType || !schedulingBehavior) {
    throw new Error(
      "[factory-graph-parity] workstation semantics are incomplete",
    );
  }
  return {
    controlRole: controlRole as FactoryGraphWorkstationSemantics["controlRole"],
    runtimeRole: runtimeRole as FactoryGraphWorkstationSemantics["runtimeRole"],
    runtimeType: runtimeType as FactoryGraphWorkstationSemantics["runtimeType"],
    schedulingBehavior:
      schedulingBehavior as FactoryGraphWorkstationSemantics["schedulingBehavior"],
  };
}

function projectParityGroups(
  groups:
    | readonly (FactoryGraphGroupRegionInput & {
        nodeIds?: readonly string[];
      })[]
    | undefined,
): FactoryGraphHostParityGroup[] {
  const projected = projectFactoryGraphGroupRegions(groups);
  return projected
    .map((group) => {
      const source = groups?.find(
        (candidate) => candidate.id.trim() === group.id,
      );
      return {
        bounds: { ...group.bounds },
        color: group.color,
        id: group.id,
        label: group.label,
        nodeIds: [...(source?.nodeIds ?? [])].sort(),
      };
    })
    .sort((left, right) => left.id.localeCompare(right.id));
}

function familyForNode(
  type: string,
  data: Record<string, unknown>,
): FactoryGraphNodeFamily {
  const kind = stringValue(data.kind);
  switch (kind) {
    case "constraint":
    case "doc":
    case "resource":
    case "worker":
    case "work-state":
    case "work-type":
    case "workstation":
      return kind;
  }
  const place = recordValue(data, "place");
  const placeKind = stringValue(recordValue(place, "kind"));
  if (placeKind === "resource") return "resource";
  if (placeKind === "work_state") return "work-state";
  switch (type) {
    case "constraint":
      return "constraint";
    case "doc":
      return "doc";
    case "resource":
      return "resource";
    case "statePosition":
      return "work-state";
    case "worker":
      return "worker";
    case "workType":
      return "work-type";
    case "workstation":
      return "workstation";
    default:
      throw new Error(
        `[factory-graph-parity] unknown semantic node type ${type}`,
      );
  }
}

function parityHandles(
  family: FactoryGraphNodeFamily,
  data: Record<string, unknown>,
): FactoryGraphHostParityHandle[] {
  const source =
    arrayValue(data.handles) ?? arrayValue(data.connectionAnchors) ?? [];
  const contract = FACTORY_GRAPH_HOST_PARITY_HANDLE_CONTRACT[family];
  const shared = new Set<string>(contract.shared);
  const excluded = new Set<string>(contract.excluded);
  return source
    .flatMap((value) => {
      if (!isRecord(value)) return [];
      const id = stringValue(value.id);
      const side = stringValue(value.side);
      const type = stringValue(value.type);
      if (
        !id ||
        (side !== "left" && side !== "right") ||
        (type !== "source" && type !== "target")
      ) {
        return [];
      }
      if (!shared.has(id) && !excluded.has(id)) {
        throw new Error(
          `[factory-graph-parity] ${family} uses unregistered handle ${id}`,
        );
      }
      if (!shared.has(id)) return [];
      return [
        {
          id,
          side: side as FactoryGraphHostParityHandle["side"],
          type: type as FactoryGraphHostParityHandle["type"],
        },
      ];
    })
    .sort((left, right) => left.id.localeCompare(right.id));
}

function parityWorkProgress(
  family: FactoryGraphNodeFamily,
  data: Record<string, unknown>,
): FactoryGraphHostParityWorkProgress {
  const tokenCount = numberValue(data.tokenCount);
  if (tokenCount !== undefined && family !== "workstation") {
    return {
      count: tokenCount,
      mode: factoryGraphWorkProgressMode(tokenCount),
    };
  }
  const activeItemLabels = arrayValue(data.activeItemLabels)?.length ?? 0;
  const executions = arrayValue(data.executions) ?? [];
  const executionItemCount = executions.reduce<number>((total, execution) => {
    if (!isRecord(execution)) return total;
    return total + (arrayValue(execution.work_items)?.length ?? 0);
  }, 0);
  const count = Math.max(activeItemLabels, executionItemCount);
  return { count, mode: factoryGraphWorkProgressMode(count) };
}

function parityFieldValue(
  node: FactoryGraphHostParityNode,
  field: FactoryGraphHostParityField,
): unknown {
  if (field === "groups") {
    throw new Error(
      "[factory-graph-parity] groups are compared at the projection level, not per node",
    );
  }
  return node[field];
}

function sharedNodeIds(
  projections: readonly FactoryGraphHostParityProjection[],
): string[] {
  const first = projections[0]?.nodes.map((node) => node.semanticNodeId) ?? [];
  return first.filter((nodeId) =>
    projections.every((projection) =>
      projection.nodes.some((node) => node.semanticNodeId === nodeId),
    ),
  );
}

function assertKnownNodeTypes(
  host: string,
  projection: FactoryGraphHostParityProjection,
): void {
  for (const node of projection.nodes) {
    if (!Object.hasOwn(FACTORY_GRAPH_NODE_TYPES, node.type)) {
      throw new Error(
        `[factory-graph-parity] ${host} uses unregistered node type ${node.type}`,
      );
    }
  }
}

function assertRenderableDimensions(
  host: string,
  projection: FactoryGraphHostParityProjection,
): void {
  for (const node of projection.nodes) {
    const values = [
      node.dimensions.height,
      node.dimensions.width,
      node.dimensions.initialHeight,
      node.dimensions.initialWidth,
      node.dimensions.measured.height,
      node.dimensions.measured.width,
    ];
    if (
      values.some((value) => value === undefined || !Number.isFinite(value))
    ) {
      throw new Error(
        `[factory-graph-parity] ${host} has incomplete React Flow measurement readiness for ${node.semanticNodeId}`,
      );
    }
  }
}

function recordValue(value: unknown, key: string): unknown {
  return isRecord(value) ? value[key] : undefined;
}

function arrayValue(value: unknown): unknown[] | undefined {
  return Array.isArray(value) ? value : undefined;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function numberValue(value: unknown): number | undefined {
  return typeof value === "number" ? value : undefined;
}

function booleanValue(value: unknown): boolean {
  return value === true;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function stableSerialize(value: unknown): string {
  return JSON.stringify(value, (_key, nested) => {
    if (!isRecord(nested)) return nested;
    return Object.fromEntries(
      Object.entries(nested).sort(([left], [right]) =>
        left.localeCompare(right),
      ),
    );
  });
}

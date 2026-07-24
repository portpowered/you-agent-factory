// biome-ignore lint/style/noExcessiveLinesPerFile: layout command inverses stay colocated for undo/redo coverage.
import {
  factoryLayoutEdgeWaypoints,
  factoryLayoutWaypointArraysEqual,
  setFactoryLayoutEdgeWaypoints,
} from "../factory-graph-layout-edge-waypoints";
import {
  type FactoryLayout,
  type FactoryLayoutPoint,
  type FactoryLayoutViewport,
  factoryLayoutNodePosition,
  moveFactoryLayoutNode,
} from "../factory-graph-layout-operations";
import {
  addFactoryLayoutGroup,
  type FactoryLayoutGroup,
  factoryLayoutGroupById,
  factoryLayoutGroupsEqual,
  moveFactoryLayoutGroupByDelta,
  removeFactoryLayoutGroup,
  updateFactoryLayoutGroup,
} from "../visual-groups/factory-graph-layout-groups";

export type FactoryLayoutGroupSnapshot =
  | { kind: "absent" }
  | { kind: "present"; group: FactoryLayoutGroup };

export type FactoryLayoutNodePositionSnapshot =
  | { kind: "absent" }
  | { kind: "present"; position: FactoryLayoutPoint };

export type FactoryLayoutCommand =
  | {
      type: "move-node";
      nodeId: string;
      from: FactoryLayoutNodePositionSnapshot;
      to: FactoryLayoutNodePositionSnapshot;
    }
  | {
      type: "move-nodes";
      moves: Array<{
        nodeId: string;
        from: FactoryLayoutNodePositionSnapshot;
        to: FactoryLayoutNodePositionSnapshot;
      }>;
    }
  | {
      type: "update-viewport";
      from: FactoryLayoutViewport | null;
      to: FactoryLayoutViewport | null;
    }
  | {
      type: "reset-layout";
      fromLayout: FactoryLayout;
      toLayout: FactoryLayout;
    }
  | {
      type: "update-edge-waypoints";
      edgeId: string;
      from: FactoryLayoutPoint[] | null;
      to: FactoryLayoutPoint[] | null;
    }
  | {
      type: "create-group";
      group: FactoryLayoutGroup;
    }
  | {
      type: "update-group";
      groupId: string;
      from: FactoryLayoutGroupSnapshot;
      to: FactoryLayoutGroupSnapshot;
    }
  | {
      type: "move-visual-group";
      groupId: string;
      fromGroup: FactoryLayoutGroupSnapshot;
      toGroup: FactoryLayoutGroupSnapshot;
      nodeMoves: Array<{
        nodeId: string;
        from: FactoryLayoutNodePositionSnapshot;
        to: FactoryLayoutNodePositionSnapshot;
      }>;
    };

export function snapshotFactoryLayoutNodePosition(
  layout: FactoryLayout,
  nodeId: string,
): FactoryLayoutNodePositionSnapshot {
  const position = factoryLayoutNodePosition(layout, nodeId);
  if (!position) {
    return { kind: "absent" };
  }

  return { kind: "present", position };
}

export function factoryLayoutPointsEqual(
  left: FactoryLayoutPoint,
  right: FactoryLayoutPoint,
): boolean {
  return left.x === right.x && left.y === right.y;
}

export function factoryLayoutViewportsEqual(
  left: FactoryLayoutViewport | null | undefined,
  right: FactoryLayoutViewport | null | undefined,
): boolean {
  if (!left && !right) {
    return true;
  }
  if (!left || !right) {
    return false;
  }

  return left.x === right.x && left.y === right.y && left.zoom === right.zoom;
}

export function factoryLayoutNodePositionSnapshotsEqual(
  left: FactoryLayoutNodePositionSnapshot,
  right: FactoryLayoutNodePositionSnapshot,
): boolean {
  if (left.kind !== right.kind) {
    return false;
  }
  if (left.kind === "absent") {
    return true;
  }

  return (
    right.kind === "present" &&
    factoryLayoutPointsEqual(left.position, right.position)
  );
}

export function createMoveFactoryLayoutNodeCommand(input: {
  layout: FactoryLayout;
  nodeId: string;
  to: FactoryLayoutPoint;
}): FactoryLayoutCommand | null {
  const from = snapshotFactoryLayoutNodePosition(input.layout, input.nodeId);
  const to: FactoryLayoutNodePositionSnapshot = {
    kind: "present",
    position: { x: input.to.x, y: input.to.y },
  };
  if (factoryLayoutNodePositionSnapshotsEqual(from, to)) {
    return null;
  }

  return {
    type: "move-node",
    nodeId: input.nodeId,
    from,
    to,
  };
}

export function createMoveFactoryLayoutNodesCommand(input: {
  layout: FactoryLayout;
  moves: Array<{ nodeId: string; to: FactoryLayoutPoint }>;
}): FactoryLayoutCommand | null {
  const moves = input.moves
    .map((move) => {
      const from = snapshotFactoryLayoutNodePosition(input.layout, move.nodeId);
      const to: FactoryLayoutNodePositionSnapshot = {
        kind: "present",
        position: { x: move.to.x, y: move.to.y },
      };
      if (factoryLayoutNodePositionSnapshotsEqual(from, to)) {
        return null;
      }

      return {
        nodeId: move.nodeId,
        from,
        to,
      };
    })
    .filter((move): move is NonNullable<typeof move> => move !== null);

  if (moves.length === 0) {
    return null;
  }

  if (moves.length === 1) {
    return {
      type: "move-node",
      nodeId: moves[0].nodeId,
      from: moves[0].from,
      to: moves[0].to,
    };
  }

  return { type: "move-nodes", moves };
}

export function createUpdateFactoryLayoutViewportCommand(input: {
  layout: FactoryLayout;
  to: FactoryLayoutViewport;
}): FactoryLayoutCommand | null {
  const from = input.layout.viewport ?? null;
  if (factoryLayoutViewportsEqual(from, input.to)) {
    return null;
  }

  return {
    type: "update-viewport",
    from,
    to: {
      x: input.to.x,
      y: input.to.y,
      zoom: input.to.zoom,
    },
  };
}

export function createUpdateFactoryLayoutEdgeWaypointsCommand(input: {
  edgeId: string;
  layout: FactoryLayout;
  to: readonly FactoryLayoutPoint[] | null;
}): FactoryLayoutCommand | null {
  const from = factoryLayoutEdgeWaypoints(input.layout, input.edgeId) ?? null;
  const to =
    input.to === null
      ? null
      : input.to.map((point) => ({ x: point.x, y: point.y }));
  if (factoryLayoutWaypointArraysEqual(from, to)) {
    return null;
  }

  return {
    type: "update-edge-waypoints",
    edgeId: input.edgeId,
    from,
    to,
  };
}

export function snapshotFactoryLayoutGroup(
  layout: FactoryLayout,
  groupId: string,
): FactoryLayoutGroupSnapshot {
  const group = factoryLayoutGroupById(layout, groupId);
  if (!group) {
    return { kind: "absent" };
  }

  return { kind: "present", group: structuredClone(group) };
}

export function factoryLayoutGroupSnapshotsEqual(
  left: FactoryLayoutGroupSnapshot,
  right: FactoryLayoutGroupSnapshot,
): boolean {
  if (left.kind !== right.kind) {
    return false;
  }
  if (left.kind === "absent") {
    return true;
  }

  return (
    right.kind === "present" &&
    factoryLayoutGroupsEqual(left.group, right.group)
  );
}

export function createCreateFactoryLayoutGroupCommand(input: {
  group: FactoryLayoutGroup;
}): FactoryLayoutCommand {
  return {
    type: "create-group",
    group: structuredClone(input.group),
  };
}

export function createDeleteFactoryLayoutGroupCommand(input: {
  groupId: string;
  layout: FactoryLayout;
}): FactoryLayoutCommand | null {
  const from = snapshotFactoryLayoutGroup(input.layout, input.groupId);
  if (from.kind === "absent") {
    return null;
  }

  return {
    type: "update-group",
    groupId: input.groupId,
    from,
    to: { kind: "absent" },
  };
}

export function createMoveFactoryLayoutVisualGroupCommand(input: {
  delta: FactoryLayoutPoint;
  groupId: string;
  layout: FactoryLayout;
  resolvedNodePositions?: ReadonlyMap<string, FactoryLayoutPoint>;
}): FactoryLayoutCommand | null {
  if (input.delta.x === 0 && input.delta.y === 0) {
    return null;
  }

  const fromGroup = snapshotFactoryLayoutGroup(input.layout, input.groupId);
  if (fromGroup.kind === "absent") {
    return null;
  }

  const nextLayout = moveFactoryLayoutGroupByDelta(
    input.layout,
    input.groupId,
    input.delta,
    input.resolvedNodePositions,
  );
  const toGroup = snapshotFactoryLayoutGroup(nextLayout, input.groupId);
  if (toGroup.kind === "absent") {
    return null;
  }

  const memberNodeIds = fromGroup.group.nodeIds ?? [];
  const nodeMoves = memberNodeIds
    .map((nodeId) => {
      const from = snapshotFactoryLayoutNodePosition(input.layout, nodeId);
      const to = snapshotFactoryLayoutNodePosition(nextLayout, nodeId);
      if (factoryLayoutNodePositionSnapshotsEqual(from, to)) {
        return null;
      }

      return { nodeId, from, to };
    })
    .filter((move): move is NonNullable<typeof move> => move !== null);

  if (
    factoryLayoutGroupSnapshotsEqual(fromGroup, toGroup) &&
    nodeMoves.length === 0
  ) {
    return null;
  }

  return {
    type: "move-visual-group",
    groupId: input.groupId,
    fromGroup,
    toGroup,
    nodeMoves,
  };
}

export function createUpdateFactoryLayoutGroupCommand(input: {
  groupId: string;
  layout: FactoryLayout;
  to: FactoryLayoutGroup;
}): FactoryLayoutCommand | null {
  const from = snapshotFactoryLayoutGroup(input.layout, input.groupId);
  const to: FactoryLayoutGroupSnapshot = {
    kind: "present",
    group: structuredClone(input.to),
  };
  if (factoryLayoutGroupSnapshotsEqual(from, to)) {
    return null;
  }

  return {
    type: "update-group",
    groupId: input.groupId,
    from,
    to,
  };
}

export function createResetFactoryLayoutCommand(input: {
  fromLayout: FactoryLayout;
  toLayout: FactoryLayout;
}): FactoryLayoutCommand | null {
  if (
    JSON.stringify(
      normalizeFactoryLayoutForCommandComparison(input.fromLayout),
    ) ===
    JSON.stringify(normalizeFactoryLayoutForCommandComparison(input.toLayout))
  ) {
    return null;
  }

  return {
    type: "reset-layout",
    fromLayout: structuredClone(input.fromLayout),
    toLayout: structuredClone(input.toLayout),
  };
}

export function invertFactoryLayoutCommand(
  command: FactoryLayoutCommand,
): FactoryLayoutCommand {
  switch (command.type) {
    case "move-node":
      return {
        type: "move-node",
        nodeId: command.nodeId,
        from: command.to,
        to: command.from,
      };
    case "move-nodes":
      return {
        type: "move-nodes",
        moves: command.moves.map((move) => ({
          nodeId: move.nodeId,
          from: move.to,
          to: move.from,
        })),
      };
    case "update-viewport":
      return {
        type: "update-viewport",
        from: command.to,
        to: command.from,
      };
    case "reset-layout":
      return {
        type: "reset-layout",
        fromLayout: structuredClone(command.toLayout),
        toLayout: structuredClone(command.fromLayout),
      };
    case "update-edge-waypoints":
      return {
        type: "update-edge-waypoints",
        edgeId: command.edgeId,
        from: command.to,
        to: command.from,
      };
    case "create-group":
      return {
        type: "update-group",
        groupId: command.group.id,
        from: { kind: "present", group: structuredClone(command.group) },
        to: { kind: "absent" },
      };
    case "update-group":
      return {
        type: "update-group",
        groupId: command.groupId,
        from: command.to,
        to: command.from,
      };
    case "move-visual-group":
      return {
        type: "move-visual-group",
        groupId: command.groupId,
        fromGroup: command.toGroup,
        toGroup: command.fromGroup,
        nodeMoves: command.nodeMoves.map((move) => ({
          nodeId: move.nodeId,
          from: move.to,
          to: move.from,
        })),
      };
  }
}

export function factoryLayoutCommandAffectedNodeIds(
  command: FactoryLayoutCommand,
): readonly string[] {
  switch (command.type) {
    case "move-node":
      return [command.nodeId];
    case "move-nodes":
      return command.moves.map((move) => move.nodeId);
    case "update-viewport":
      return [];
    case "reset-layout": {
      const nodeIds = new Set<string>();
      for (const node of command.fromLayout.nodes ?? []) {
        nodeIds.add(node.id);
      }
      for (const node of command.toLayout.nodes ?? []) {
        nodeIds.add(node.id);
      }
      return [...nodeIds];
    }
    case "update-edge-waypoints":
      return [];
    case "create-group":
      return [];
    case "update-group":
      return [];
    case "move-visual-group":
      return command.nodeMoves.map((move) => move.nodeId);
  }
}

export function factoryLayoutCommandReferencesDeletedNodeIds(
  command: FactoryLayoutCommand,
  activeNodeIds: ReadonlySet<string>,
): boolean {
  return factoryLayoutCommandAffectedNodeIds(command).some(
    (nodeId) => !activeNodeIds.has(nodeId),
  );
}

export function removeFactoryLayoutNodeEntry(
  layout: FactoryLayout,
  nodeId: string,
): FactoryLayout {
  const nodes = (layout.nodes ?? []).filter((entry) => entry.id !== nodeId);
  return {
    ...layout,
    nodes,
  };
}

export function applyFactoryLayoutNodePositionSnapshot(
  layout: FactoryLayout,
  nodeId: string,
  snapshot: FactoryLayoutNodePositionSnapshot,
): FactoryLayout {
  if (snapshot.kind === "absent") {
    return removeFactoryLayoutNodeEntry(layout, nodeId);
  }

  return moveFactoryLayoutNode(layout, nodeId, snapshot.position);
}

export function applyFactoryLayoutCommand(
  layout: FactoryLayout,
  command: FactoryLayoutCommand,
): FactoryLayout {
  switch (command.type) {
    case "move-node":
      return applyFactoryLayoutNodePositionSnapshot(
        layout,
        command.nodeId,
        command.to,
      );
    case "move-nodes": {
      let nextLayout = layout;
      for (const move of command.moves) {
        nextLayout = applyFactoryLayoutNodePositionSnapshot(
          nextLayout,
          move.nodeId,
          move.to,
        );
      }
      return nextLayout;
    }
    case "update-viewport":
      if (command.to === null) {
        return {
          ...layout,
          viewport: undefined,
        };
      }
      return {
        ...layout,
        viewport: {
          x: command.to.x,
          y: command.to.y,
          zoom: command.to.zoom,
        },
      };
    case "reset-layout":
      return structuredClone(command.toLayout);
    case "update-edge-waypoints":
      return setFactoryLayoutEdgeWaypoints(layout, command.edgeId, command.to);
    case "create-group":
      return addFactoryLayoutGroup(layout, command.group);
    case "update-group": {
      if (command.to.kind === "absent") {
        return removeFactoryLayoutGroup(layout, command.groupId);
      }

      const nextGroup = command.to.group;
      if (factoryLayoutGroupById(layout, command.groupId)) {
        return updateFactoryLayoutGroup(
          layout,
          command.groupId,
          () => nextGroup,
        );
      }

      return addFactoryLayoutGroup(layout, nextGroup);
    }
    case "move-visual-group": {
      let nextLayout = layout;
      const toGroup = command.toGroup;
      if (toGroup.kind === "present") {
        nextLayout = updateFactoryLayoutGroup(
          nextLayout,
          command.groupId,
          () => toGroup.group,
        );
      }

      for (const move of command.nodeMoves) {
        nextLayout = applyFactoryLayoutNodePositionSnapshot(
          nextLayout,
          move.nodeId,
          move.to,
        );
      }

      return nextLayout;
    }
  }
}

function normalizeFactoryLayoutForCommandComparison(layout: FactoryLayout) {
  return {
    edges: layout.edges ?? [],
    groups: layout.groups ?? [],
    nodes: [...(layout.nodes ?? [])].sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    preferences: layout.preferences ?? null,
    schemaVersion: layout.schemaVersion ?? null,
    viewport: layout.viewport ?? null,
  };
}

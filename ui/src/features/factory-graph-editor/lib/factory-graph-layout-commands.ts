import {
  type FactoryLayout,
  type FactoryLayoutPoint,
  type FactoryLayoutViewport,
  factoryLayoutNodePosition,
  moveFactoryLayoutNode,
} from "./factory-graph-layout-operations";

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

export function createResetFactoryLayoutCommand(input: {
  fromLayout: FactoryLayout;
  toLayout: FactoryLayout;
}): FactoryLayoutCommand | null {
  if (
    JSON.stringify(normalizeFactoryLayoutForCommandComparison(input.fromLayout)) ===
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

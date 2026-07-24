// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: layout command inverse scenarios stay grouped around shared fixtures.
import { describe, expect, it } from "vitest";
import {
  addFactoryLayoutEdgeWaypoint,
  factoryLayoutEdgeWaypoints,
  moveFactoryLayoutEdgeWaypoint,
  removeFactoryLayoutEdgeWaypoint,
  setFactoryLayoutEdgeWaypoints,
} from "../factory-graph-layout-edge-waypoints";
import {
  createDefaultFactoryLayout,
  factoryLayoutNodePosition,
  moveFactoryLayoutNode,
} from "../factory-graph-layout-operations";
import {
  addFactoryLayoutGroup,
  addNodeToFactoryLayoutGroup,
  createFactoryLayoutGroup,
  defaultFactoryLayoutGroupBounds,
  factoryLayoutGroupById,
} from "../visual-groups/factory-graph-layout-groups";
import {
  applyFactoryLayoutCommand,
  createDeleteFactoryLayoutGroupCommand,
  createMoveFactoryLayoutNodeCommand,
  createMoveFactoryLayoutNodesCommand,
  createMoveFactoryLayoutVisualGroupCommand,
  createResetFactoryLayoutCommand,
  createUpdateFactoryLayoutEdgeWaypointsCommand,
  createUpdateFactoryLayoutViewportCommand,
  type FactoryLayoutCommand,
  factoryLayoutCommandAffectedNodeIds,
  factoryLayoutCommandReferencesDeletedNodeIds,
  invertFactoryLayoutCommand,
} from "./factory-graph-layout-commands";

const EDGE_ID = "workstation-output:workstation:draft->work-state:story:done";

function requireCommand(
  command: FactoryLayoutCommand | null,
): FactoryLayoutCommand {
  expect(command).not.toBeNull();
  if (!command) {
    throw new Error("Expected layout command to be created.");
  }
  return command;
}

describe("factory graph layout commands", () => {
  it("creates and inverts single-node movement without whole-document snapshots", () => {
    const layout = moveFactoryLayoutNode(
      createDefaultFactoryLayout(),
      "worker:writer",
      {
        x: 40,
        y: 80,
      },
    );
    const command = requireCommand(
      createMoveFactoryLayoutNodeCommand({
        layout,
        nodeId: "worker:writer",
        to: { x: 120, y: 160 },
      }),
    );

    expect(command).toEqual({
      type: "move-node",
      nodeId: "worker:writer",
      from: { kind: "present", position: { x: 40, y: 80 } },
      to: { kind: "present", position: { x: 120, y: 160 } },
    });

    const nextLayout = applyFactoryLayoutCommand(layout, command);
    const undoneLayout = applyFactoryLayoutCommand(
      nextLayout,
      invertFactoryLayoutCommand(command),
    );

    expect(factoryLayoutNodePosition(undoneLayout, "worker:writer")).toEqual({
      x: 40,
      y: 80,
    });
  });

  it("restores auto-layout fallback when undoing the first saved move", () => {
    const layout = createDefaultFactoryLayout();
    const command = requireCommand(
      createMoveFactoryLayoutNodeCommand({
        layout,
        nodeId: "worker:writer",
        to: { x: 120, y: 160 },
      }),
    );

    const nextLayout = applyFactoryLayoutCommand(layout, command);
    const undoneLayout = applyFactoryLayoutCommand(
      nextLayout,
      invertFactoryLayoutCommand(command),
    );

    expect(
      factoryLayoutNodePosition(undoneLayout, "worker:writer"),
    ).toBeUndefined();
  });

  it("creates and inverts multi-node movement commands", () => {
    const layout = createDefaultFactoryLayout();
    const command = requireCommand(
      createMoveFactoryLayoutNodesCommand({
        layout,
        moves: [
          { nodeId: "worker:writer", to: { x: 10, y: 20 } },
          { nodeId: "workstation:draft", to: { x: 30, y: 40 } },
        ],
      }),
    );

    expect(command.type).toBe("move-nodes");

    const nextLayout = applyFactoryLayoutCommand(layout, command);
    const undoneLayout = applyFactoryLayoutCommand(
      nextLayout,
      invertFactoryLayoutCommand(command),
    );

    expect(
      factoryLayoutNodePosition(undoneLayout, "worker:writer"),
    ).toBeUndefined();
    expect(
      factoryLayoutNodePosition(undoneLayout, "workstation:draft"),
    ).toBeUndefined();
  });

  it("creates and inverts viewport updates", () => {
    const layout = createDefaultFactoryLayout();
    const command = requireCommand(
      createUpdateFactoryLayoutViewportCommand({
        layout,
        to: { x: 120, y: 80, zoom: 1.25 },
      }),
    );

    const nextLayout = applyFactoryLayoutCommand(layout, command);
    const undoneLayout = applyFactoryLayoutCommand(
      nextLayout,
      invertFactoryLayoutCommand(command),
    );

    expect(nextLayout.viewport).toEqual({ x: 120, y: 80, zoom: 1.25 });
    expect(undoneLayout.viewport).toBeUndefined();
  });

  it("creates and inverts reset layout commands", () => {
    const fromLayout = moveFactoryLayoutNode(
      createDefaultFactoryLayout(),
      "worker:writer",
      {
        x: 40,
        y: 80,
      },
    );
    const toLayout = createDefaultFactoryLayout();
    const command = requireCommand(
      createResetFactoryLayoutCommand({ fromLayout, toLayout }),
    );

    const nextLayout = applyFactoryLayoutCommand(fromLayout, command);
    const undoneLayout = applyFactoryLayoutCommand(
      nextLayout,
      invertFactoryLayoutCommand(command),
    );

    expect(
      factoryLayoutNodePosition(nextLayout, "worker:writer"),
    ).toBeUndefined();
    expect(factoryLayoutNodePosition(undoneLayout, "worker:writer")).toEqual({
      x: 40,
      y: 80,
    });
  });

  it("creates and inverts edge waypoint updates without whole-document snapshots", () => {
    const layout = addFactoryLayoutEdgeWaypoint(
      addFactoryLayoutEdgeWaypoint(createDefaultFactoryLayout(), EDGE_ID, {
        x: 10,
        y: 20,
      }),
      EDGE_ID,
      { x: 30, y: 40 },
    );
    const command = requireCommand(
      createUpdateFactoryLayoutEdgeWaypointsCommand({
        edgeId: EDGE_ID,
        layout,
        to: [
          { x: 10, y: 20 },
          { x: 50, y: 60 },
        ],
      }),
    );

    expect(command).toEqual({
      type: "update-edge-waypoints",
      edgeId: EDGE_ID,
      from: [
        { x: 10, y: 20 },
        { x: 30, y: 40 },
      ],
      to: [
        { x: 10, y: 20 },
        { x: 50, y: 60 },
      ],
    });

    const nextLayout = applyFactoryLayoutCommand(layout, command);
    const undoneLayout = applyFactoryLayoutCommand(
      nextLayout,
      invertFactoryLayoutCommand(command),
    );

    expect(factoryLayoutEdgeWaypoints(nextLayout, EDGE_ID)).toEqual([
      { x: 10, y: 20 },
      { x: 50, y: 60 },
    ]);
    expect(factoryLayoutEdgeWaypoints(undoneLayout, EDGE_ID)).toEqual([
      { x: 10, y: 20 },
      { x: 30, y: 40 },
    ]);
  });

  it("restores generated routing when undoing the first authored waypoint", () => {
    const layout = createDefaultFactoryLayout();
    const command = requireCommand(
      createUpdateFactoryLayoutEdgeWaypointsCommand({
        edgeId: EDGE_ID,
        layout,
        to: [{ x: 120, y: 160 }],
      }),
    );

    const nextLayout = applyFactoryLayoutCommand(layout, command);
    const undoneLayout = applyFactoryLayoutCommand(
      nextLayout,
      invertFactoryLayoutCommand(command),
    );

    expect(factoryLayoutEdgeWaypoints(nextLayout, EDGE_ID)).toEqual([
      { x: 120, y: 160 },
    ]);
    expect(factoryLayoutEdgeWaypoints(undoneLayout, EDGE_ID)).toBeUndefined();
  });

  it("creates waypoint removal commands that preserve remaining waypoint order", () => {
    const layout = setFactoryLayoutEdgeWaypoints(
      createDefaultFactoryLayout(),
      EDGE_ID,
      [
        { x: 10, y: 20 },
        { x: 30, y: 40 },
        { x: 50, y: 60 },
      ],
    );
    const nextLayout = removeFactoryLayoutEdgeWaypoint(layout, EDGE_ID, 1);
    const command = requireCommand(
      createUpdateFactoryLayoutEdgeWaypointsCommand({
        edgeId: EDGE_ID,
        layout,
        to: factoryLayoutEdgeWaypoints(nextLayout, EDGE_ID) ?? null,
      }),
    );

    const appliedLayout = applyFactoryLayoutCommand(layout, command);
    const undoneLayout = applyFactoryLayoutCommand(
      appliedLayout,
      invertFactoryLayoutCommand(command),
    );

    expect(factoryLayoutEdgeWaypoints(appliedLayout, EDGE_ID)).toEqual([
      { x: 10, y: 20 },
      { x: 50, y: 60 },
    ]);
    expect(factoryLayoutEdgeWaypoints(undoneLayout, EDGE_ID)).toEqual([
      { x: 10, y: 20 },
      { x: 30, y: 40 },
      { x: 50, y: 60 },
    ]);
  });

  it("skips waypoint commands when the target matches the current layout", () => {
    const layout = moveFactoryLayoutEdgeWaypoint(
      addFactoryLayoutEdgeWaypoint(createDefaultFactoryLayout(), EDGE_ID, {
        x: 10,
        y: 20,
      }),
      EDGE_ID,
      0,
      { x: 30, y: 40 },
    );

    expect(
      createUpdateFactoryLayoutEdgeWaypointsCommand({
        edgeId: EDGE_ID,
        layout,
        to: factoryLayoutEdgeWaypoints(layout, EDGE_ID) ?? null,
      }),
    ).toBeNull();
  });

  it("creates and inverts visual group move commands with member nodes", () => {
    const layout = moveFactoryLayoutNode(
      addFactoryLayoutGroup(
        createDefaultFactoryLayout(),
        createFactoryLayoutGroup({
          bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
          id: "group-1",
          layout: createDefaultFactoryLayout(),
        }),
      ),
      "workstation:draft",
      { x: 40, y: 60 },
    );
    const withMember = addNodeToFactoryLayoutGroup(
      layout,
      "group-1",
      "workstation:draft",
    );
    const command = requireCommand(
      createMoveFactoryLayoutVisualGroupCommand({
        delta: { x: 12, y: 8 },
        groupId: "group-1",
        layout: withMember,
      }),
    );

    expect(command.type).toBe("move-visual-group");

    const nextLayout = applyFactoryLayoutCommand(withMember, command);
    const undoneLayout = applyFactoryLayoutCommand(
      nextLayout,
      invertFactoryLayoutCommand(command),
    );

    expect(factoryLayoutGroupById(nextLayout, "group-1")?.bounds).toEqual({
      height: 320,
      width: 480,
      x: -228,
      y: -152,
    });
    expect(factoryLayoutNodePosition(nextLayout, "workstation:draft")).toEqual({
      x: 52,
      y: 68,
    });
    expect(factoryLayoutGroupById(undoneLayout, "group-1")?.bounds).toEqual(
      factoryLayoutGroupById(withMember, "group-1")?.bounds,
    );
    expect(
      factoryLayoutNodePosition(undoneLayout, "workstation:draft"),
    ).toEqual({
      x: 40,
      y: 60,
    });
    expect(factoryLayoutCommandAffectedNodeIds(command)).toEqual([
      "workstation:draft",
    ]);
  });

  it("creates and inverts visual group delete commands", () => {
    const layout = addFactoryLayoutGroup(
      moveFactoryLayoutNode(createDefaultFactoryLayout(), "workstation:draft", {
        x: 40,
        y: 60,
      }),
      createFactoryLayoutGroup({
        bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
        id: "group-1",
        layout: createDefaultFactoryLayout(),
      }),
    );
    const command = requireCommand(
      createDeleteFactoryLayoutGroupCommand({
        groupId: "group-1",
        layout,
      }),
    );

    const nextLayout = applyFactoryLayoutCommand(layout, command);
    const undoneLayout = applyFactoryLayoutCommand(
      nextLayout,
      invertFactoryLayoutCommand(command),
    );

    expect(nextLayout.groups).toBeUndefined();
    expect(factoryLayoutNodePosition(nextLayout, "workstation:draft")).toEqual({
      x: 40,
      y: 60,
    });
    expect(factoryLayoutGroupById(undoneLayout, "group-1")?.id).toBe("group-1");
  });

  it("flags commands that reference deleted graph ids", () => {
    const command = requireCommand(
      createMoveFactoryLayoutNodeCommand({
        layout: createDefaultFactoryLayout(),
        nodeId: "worker:writer",
        to: { x: 10, y: 20 },
      }),
    );

    expect(
      factoryLayoutCommandReferencesDeletedNodeIds(
        command,
        new Set(["worker:writer"]),
      ),
    ).toBe(false);
    expect(
      factoryLayoutCommandReferencesDeletedNodeIds(
        command,
        new Set(["workstation:draft"]),
      ),
    ).toBe(true);
  });
});

describe("factory graph layout command helpers", () => {
  it("clears viewport when applying a null viewport command target", () => {
    const layout = {
      ...createDefaultFactoryLayout(),
      viewport: { x: 10, y: 20, zoom: 1.5 },
    };
    const command: FactoryLayoutCommand = {
      type: "update-viewport",
      from: layout.viewport ?? null,
      to: null,
    };

    expect(applyFactoryLayoutCommand(layout, command).viewport).toBeUndefined();
  });

  it("collects affected node ids for reset-layout commands", () => {
    const fromLayout = moveFactoryLayoutNode(
      createDefaultFactoryLayout(),
      "worker:writer",
      {
        x: 40,
        y: 80,
      },
    );
    const command = requireCommand(
      createResetFactoryLayoutCommand({
        fromLayout,
        toLayout: createDefaultFactoryLayout(),
      }),
    );

    expect(factoryLayoutCommandAffectedNodeIds(command)).toEqual([
      "worker:writer",
    ]);
  });
});

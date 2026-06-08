import { describe, expect, it } from "vitest";

import {
  applyFactoryLayoutCommand,
  createMoveFactoryLayoutNodeCommand,
  createMoveFactoryLayoutNodesCommand,
  createResetFactoryLayoutCommand,
  createUpdateFactoryLayoutViewportCommand,
  factoryLayoutCommandAffectedNodeIds,
  factoryLayoutCommandReferencesDeletedNodeIds,
  invertFactoryLayoutCommand,
  type FactoryLayoutCommand,
} from "./factory-graph-layout-commands";
import {
  createDefaultFactoryLayout,
  factoryLayoutNodePosition,
  moveFactoryLayoutNode,
} from "../factory-graph-layout-operations";

function requireCommand(command: FactoryLayoutCommand | null): FactoryLayoutCommand {
  expect(command).not.toBeNull();
  if (!command) {
    throw new Error("Expected layout command to be created.");
  }
  return command;
}

describe("factory graph layout commands", () => {
  it("creates and inverts single-node movement without whole-document snapshots", () => {
    const layout = moveFactoryLayoutNode(createDefaultFactoryLayout(), "worker:writer", {
      x: 40,
      y: 80,
    });
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

    expect(factoryLayoutNodePosition(undoneLayout, "worker:writer")).toBeUndefined();
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

    expect(factoryLayoutNodePosition(undoneLayout, "worker:writer")).toBeUndefined();
    expect(factoryLayoutNodePosition(undoneLayout, "workstation:draft")).toBeUndefined();
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
    const fromLayout = moveFactoryLayoutNode(createDefaultFactoryLayout(), "worker:writer", {
      x: 40,
      y: 80,
    });
    const toLayout = createDefaultFactoryLayout();
    const command = requireCommand(
      createResetFactoryLayoutCommand({ fromLayout, toLayout }),
    );

    const nextLayout = applyFactoryLayoutCommand(fromLayout, command);
    const undoneLayout = applyFactoryLayoutCommand(
      nextLayout,
      invertFactoryLayoutCommand(command),
    );

    expect(factoryLayoutNodePosition(nextLayout, "worker:writer")).toBeUndefined();
    expect(factoryLayoutNodePosition(undoneLayout, "worker:writer")).toEqual({
      x: 40,
      y: 80,
    });
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
      factoryLayoutCommandReferencesDeletedNodeIds(command, new Set(["worker:writer"])),
    ).toBe(false);
    expect(
      factoryLayoutCommandReferencesDeletedNodeIds(command, new Set(["workstation:draft"])),
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
    const fromLayout = moveFactoryLayoutNode(createDefaultFactoryLayout(), "worker:writer", {
      x: 40,
      y: 80,
    });
    const command = requireCommand(
      createResetFactoryLayoutCommand({
        fromLayout,
        toLayout: createDefaultFactoryLayout(),
      }),
    );

    expect(factoryLayoutCommandAffectedNodeIds(command)).toEqual(["worker:writer"]);
  });
});

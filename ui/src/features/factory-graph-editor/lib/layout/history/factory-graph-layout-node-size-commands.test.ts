import { describe, expect, it } from "vitest";
import { factoryLayoutNodeSize } from "../factory-graph-layout-operations";
import {
  applyFactoryLayoutCommand,
  createFitFactoryLayoutNodeCommand,
  createResetFactoryLayoutNodeSizeCommand,
  createResizeFactoryLayoutNodeCommand,
  type FactoryLayoutCommand,
  factoryLayoutCommandAffectedNodeIds,
  invertFactoryLayoutCommand,
} from "./factory-graph-layout-commands";

function requireCommand(
  command: FactoryLayoutCommand | null,
): FactoryLayoutCommand {
  expect(command).not.toBeNull();
  if (!command) {
    throw new Error("Expected layout command to be created.");
  }
  return command;
}

describe("factory graph node-size commands", () => {
  it("records one node-size command and restores the exact authored node", () => {
    const layout = {
      schemaVersion: 1,
      nodes: [
        {
          id: "workstation:draft",
          locked: true,
          position: { x: 40, y: 80 },
          size: { height: 240, width: 240 },
        },
      ],
    };
    const command = requireCommand(
      createResizeFactoryLayoutNodeCommand({
        family: "workstation",
        layout,
        nodeId: "workstation:draft",
        position: { x: 40, y: 80 },
        requestedDimensions: { height: 400, width: 9999 },
      }),
    );

    expect(command.type).toBe("update-node-size");
    const resized = applyFactoryLayoutCommand(layout, command);
    expect(factoryLayoutNodeSize(resized, "workstation:draft")).toEqual({
      height: 400,
      width: 520,
    });

    const undone = applyFactoryLayoutCommand(
      resized,
      invertFactoryLayoutCommand(command),
    );
    expect(undone.nodes).toEqual(layout.nodes);
    expect(factoryLayoutCommandAffectedNodeIds(command)).toEqual([
      "workstation:draft",
    ]);
  });

  it("records fit and reset as one-step authored-size commands", () => {
    const layout = {
      schemaVersion: 1,
      nodes: [
        {
          id: "worker:writer",
          position: { x: 10, y: 20 },
        },
      ],
    };
    const fitCommand = requireCommand(
      createFitFactoryLayoutNodeCommand({
        content: "a-long-unbroken-worker-identifier-that-needs-safe-fitting",
        family: "worker",
        layout,
        nodeId: "worker:writer",
        position: { x: 10, y: 20 },
      }),
    );
    const fitted = applyFactoryLayoutCommand(layout, fitCommand);
    expect(factoryLayoutNodeSize(fitted, "worker:writer")).toEqual({
      height: 58,
      width: 360,
    });

    const resetCommand = requireCommand(
      createResetFactoryLayoutNodeSizeCommand({
        layout: fitted,
        nodeId: "worker:writer",
      }),
    );
    const reset = applyFactoryLayoutCommand(fitted, resetCommand);
    expect(factoryLayoutNodeSize(reset, "worker:writer")).toBeUndefined();
    expect(reset.nodes).toEqual(layout.nodes);

    const restored = applyFactoryLayoutCommand(
      reset,
      invertFactoryLayoutCommand(resetCommand),
    );
    expect(factoryLayoutNodeSize(restored, "worker:writer")).toEqual({
      height: 58,
      width: 360,
    });
  });
});

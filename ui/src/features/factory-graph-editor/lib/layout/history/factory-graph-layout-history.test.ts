import { describe, expect, it } from "vitest";
import { createDefaultFactoryLayout } from "../factory-graph-layout-operations";
import {
  applyFactoryLayoutCommand,
  createMoveFactoryLayoutNodeCommand,
  createUpdateFactoryLayoutViewportCommand,
  type FactoryLayoutCommand,
} from "./factory-graph-layout-commands";
import {
  canRedoFactoryLayoutHistory,
  canUndoFactoryLayoutHistory,
  clearFactoryLayoutHistoryState,
  createFactoryLayoutHistoryState,
  FACTORY_LAYOUT_HISTORY_LIMIT,
  pruneFactoryLayoutHistoryForNodeIds,
  pushFactoryLayoutHistoryCommand,
  redoFactoryLayoutHistory,
  undoFactoryLayoutHistory,
} from "./factory-graph-layout-history";

function requireCommand(
  command: FactoryLayoutCommand | null,
): FactoryLayoutCommand {
  expect(command).not.toBeNull();
  if (!command) {
    throw new Error("Expected layout command to be created.");
  }
  return command;
}

describe("factory graph layout history", () => {
  it("pushes commands and supports undo and redo", () => {
    let layout = createDefaultFactoryLayout();
    let history = createFactoryLayoutHistoryState();
    const moveCommand = requireCommand(
      createMoveFactoryLayoutNodeCommand({
        layout,
        nodeId: "worker:writer",
        to: { x: 40, y: 80 },
      }),
    );

    history = pushFactoryLayoutHistoryCommand(history, moveCommand);
    layout = applyFactoryLayoutCommand(layout, moveCommand);

    expect(canUndoFactoryLayoutHistory(history)).toBe(true);
    expect(canRedoFactoryLayoutHistory(history)).toBe(false);

    const undoResult = undoFactoryLayoutHistory(history, layout);
    history = undoResult.history;
    layout = undoResult.layout;

    expect(canUndoFactoryLayoutHistory(history)).toBe(false);
    expect(canRedoFactoryLayoutHistory(history)).toBe(true);
    expect(layout.nodes ?? []).toHaveLength(0);

    const redoResult = redoFactoryLayoutHistory(history, layout);
    history = redoResult.history;
    layout = redoResult.layout;

    expect(layout.nodes?.[0]?.position).toEqual({ x: 40, y: 80 });
  });

  it("prunes history entries that target deleted graph ids", () => {
    const layout = createDefaultFactoryLayout();
    const moveCommand = requireCommand(
      createMoveFactoryLayoutNodeCommand({
        layout,
        nodeId: "worker:writer",
        to: { x: 40, y: 80 },
      }),
    );
    const viewportCommand = requireCommand(
      createUpdateFactoryLayoutViewportCommand({
        layout,
        to: { x: 10, y: 20, zoom: 1 },
      }),
    );
    const history = pushFactoryLayoutHistoryCommand(
      pushFactoryLayoutHistoryCommand(
        createFactoryLayoutHistoryState(),
        moveCommand,
      ),
      viewportCommand,
    );

    const pruned = pruneFactoryLayoutHistoryForNodeIds(
      history,
      new Set(["workstation:draft"]),
    );

    expect(pruned.past).toEqual([viewportCommand]);
  });

  it("returns null commands when undo or redo stacks are empty", () => {
    const layout = createDefaultFactoryLayout();
    const history = createFactoryLayoutHistoryState();

    expect(undoFactoryLayoutHistory(history, layout)).toEqual({
      command: null,
      history,
      layout,
    });
    expect(redoFactoryLayoutHistory(history, layout)).toEqual({
      command: null,
      history,
      layout,
    });
  });

  it("drops the oldest history entry after the limit is exceeded", () => {
    let history = createFactoryLayoutHistoryState();
    const layout = createDefaultFactoryLayout();

    for (let index = 0; index < FACTORY_LAYOUT_HISTORY_LIMIT + 1; index += 1) {
      const command = requireCommand(
        createMoveFactoryLayoutNodeCommand({
          layout,
          nodeId: "worker:writer",
          to: { x: index, y: index },
        }),
      );
      history = pushFactoryLayoutHistoryCommand(history, command);
    }

    expect(history.past).toHaveLength(FACTORY_LAYOUT_HISTORY_LIMIT);
    expect(history.past[0]?.type).toBe("move-node");
    expect(history.past.at(-1)?.type).toBe("move-node");
  });

  it("clears undo and redo stacks", () => {
    const cleared = clearFactoryLayoutHistoryState();

    expect(cleared).toEqual({ future: [], past: [] });
    expect(canUndoFactoryLayoutHistory(cleared)).toBe(false);
    expect(canRedoFactoryLayoutHistory(cleared)).toBe(false);
  });
});

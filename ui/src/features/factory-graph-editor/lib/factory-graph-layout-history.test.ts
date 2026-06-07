import { describe, expect, it } from "vitest";

import {
  applyFactoryLayoutCommand,
  createMoveFactoryLayoutNodeCommand,
  createUpdateFactoryLayoutViewportCommand,
} from "./factory-graph-layout-commands";
import {
  canRedoFactoryLayoutHistory,
  canUndoFactoryLayoutHistory,
  createFactoryLayoutHistoryState,
  pruneFactoryLayoutHistoryForNodeIds,
  pushFactoryLayoutHistoryCommand,
  redoFactoryLayoutHistory,
  undoFactoryLayoutHistory,
} from "./factory-graph-layout-history";
import { createDefaultFactoryLayout } from "./factory-graph-layout-operations";

describe("factory graph layout history", () => {
  it("pushes commands and supports undo and redo", () => {
    let layout = createDefaultFactoryLayout();
    let history = createFactoryLayoutHistoryState();
    const moveCommand = createMoveFactoryLayoutNodeCommand({
      layout,
      nodeId: "worker:writer",
      to: { x: 40, y: 80 },
    })!;

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
    const moveCommand = createMoveFactoryLayoutNodeCommand({
      layout,
      nodeId: "worker:writer",
      to: { x: 40, y: 80 },
    })!;
    const viewportCommand = createUpdateFactoryLayoutViewportCommand({
      layout,
      to: { x: 10, y: 20, zoom: 1 },
    })!;
    const history = pushFactoryLayoutHistoryCommand(
      pushFactoryLayoutHistoryCommand(createFactoryLayoutHistoryState(), moveCommand),
      viewportCommand,
    );

    const pruned = pruneFactoryLayoutHistoryForNodeIds(
      history,
      new Set(["workstation:draft"]),
    );

    expect(pruned.past).toEqual([viewportCommand]);
  });
});

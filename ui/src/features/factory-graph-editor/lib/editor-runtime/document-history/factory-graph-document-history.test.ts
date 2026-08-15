import { describe, expect, it } from "vitest";

import { createEmptyFactoryGraphDraft } from "../../draft/factory-graph-draft-types";
import {
  createDefaultFactoryLayout,
  moveFactoryLayoutNode,
} from "../../layout/factory-graph-layout-operations";
import {
  canRedoFactoryGraphDocumentHistory,
  canUndoFactoryGraphDocumentHistory,
  createFactoryGraphDocumentHistoryState,
  recordFactoryGraphDocumentTransaction,
  redoFactoryGraphDocumentHistory,
  undoFactoryGraphDocumentHistory,
} from "./factory-graph-document-history";

describe("factory graph document history", () => {
  it("restores mixed domain snapshots in strict LIFO order", () => {
    const initialDraft = createEmptyFactoryGraphDraft();
    const initialLayout = createDefaultFactoryLayout();
    let history = createFactoryGraphDocumentHistoryState({
      draft: initialDraft,
      layout: initialLayout,
    });

    const topologyDraft = structuredClone(initialDraft);
    topologyDraft.removals.resources.push("gpu");
    history = recordFactoryGraphDocumentTransaction(history, {
      draft: topologyDraft,
      layout: initialLayout,
    });

    const layout = moveFactoryLayoutNode(initialLayout, "worker:writer", {
      x: 120,
      y: 80,
    });
    history = recordFactoryGraphDocumentTransaction(history, {
      draft: topologyDraft,
      layout,
    });

    expect(canUndoFactoryGraphDocumentHistory(history)).toBe(true);
    expect(canRedoFactoryGraphDocumentHistory(history)).toBe(false);

    const undoneLayout = undoFactoryGraphDocumentHistory(history);
    history = undoneLayout.history;
    expect(undoneLayout.snapshot?.draft).toEqual(topologyDraft);
    expect(undoneLayout.snapshot?.layout).toEqual(initialLayout);

    const undoneTopology = undoFactoryGraphDocumentHistory(history);
    history = undoneTopology.history;
    expect(undoneTopology.snapshot?.draft).toEqual(initialDraft);
    expect(undoneTopology.snapshot?.layout).toEqual(initialLayout);

    const redoneTopology = redoFactoryGraphDocumentHistory(history);
    history = redoneTopology.history;
    expect(redoneTopology.snapshot?.draft).toEqual(topologyDraft);
    expect(redoneTopology.snapshot?.layout).toEqual(initialLayout);

    const redoneLayout = redoFactoryGraphDocumentHistory(history);
    expect(redoneLayout.snapshot?.draft).toEqual(topologyDraft);
    expect(redoneLayout.snapshot?.layout).toEqual(layout);
  });

  it("drops redo after a new transaction and ignores no-op snapshots", () => {
    const initial = {
      draft: createEmptyFactoryGraphDraft(),
      layout: createDefaultFactoryLayout(),
    };
    let history = createFactoryGraphDocumentHistoryState(initial);
    const changedDraft = structuredClone(initial.draft);
    changedDraft.additions.resources.push({ capacity: 1, name: "cache" });

    history = recordFactoryGraphDocumentTransaction(history, {
      ...initial,
      draft: changedDraft,
    });
    history = undoFactoryGraphDocumentHistory(history).history;
    expect(canRedoFactoryGraphDocumentHistory(history)).toBe(true);

    const secondDraft = structuredClone(initial.draft);
    secondDraft.additions.resources.push({ capacity: 1, name: "queue" });
    history = recordFactoryGraphDocumentTransaction(history, {
      ...initial,
      draft: secondDraft,
    });

    expect(canRedoFactoryGraphDocumentHistory(history)).toBe(false);
    expect(
      recordFactoryGraphDocumentTransaction(history, history.present),
    ).toBe(history);
  });
});

import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { useFactoryGraphLayoutDraftState } from "./factory-graph-layout-draft-hook";
import { baseFactoryDefinition } from "../lib/factory-graph-draft.test-helpers";
import {
  factoryLayoutNodePosition,
  moveFactoryLayoutNode,
} from "../lib/factory-graph-layout-operations";

describe("useFactoryGraphLayoutDraftState", () => {
  it("records layout commands and supports undo and redo", () => {
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: baseFactoryDefinition,
        factoryDocumentScopeKey: "session-1",
      }),
    );

    act(() => {
      result.current.moveNode("workstation:draft", { x: 120, y: 160 });
    });

    expect(result.current.canUndoLayout).toBe(true);
    expect(factoryLayoutNodePosition(result.current.layout, "workstation:draft")).toEqual({
      x: 120,
      y: 160,
    });

    act(() => {
      result.current.undoLayout();
    });

    expect(result.current.canRedoLayout).toBe(true);
    expect(factoryLayoutNodePosition(result.current.layout, "workstation:draft")).toBeUndefined();

    act(() => {
      result.current.redoLayout();
    });

    expect(factoryLayoutNodePosition(result.current.layout, "workstation:draft")).toEqual({
      x: 120,
      y: 160,
    });
  });

  it("clears history when discarding layout without recording a reset command", () => {
    const layoutDocument = {
      ...baseFactoryDefinition,
      layout: moveFactoryLayoutNode(baseFactoryDefinition.layout ?? {}, "workstation:draft", {
        x: 40,
        y: 80,
      }),
    };
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: layoutDocument,
        factoryDocumentScopeKey: "session-2",
      }),
    );

    act(() => {
      result.current.moveNode("workstation:draft", { x: 120, y: 160 });
      result.current.resetLayout({ recordHistory: false });
    });

    expect(result.current.canUndoLayout).toBe(false);
    expect(result.current.canRedoLayout).toBe(false);
  });
});

import { act } from "@testing-library/react";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import { renderEditableFactoryGraphHook } from "../../../testing/editable-factory-graph-hook-test-helpers";

const documentFactory: CurrentFactoryDocument = {
  name: "History Factory",
  version: {
    logical: "9",
    physical: "2026-05-31T01:00:00Z",
  },
  workTypes: [
    {
      name: "story",
      states: [
        { name: "queued", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workers: [{ model: "gpt-5", name: "writer", type: "MODEL_WORKER" }],
  workstations: [
    {
      body: "Document plane baseline.",
      inputs: [{ state: "queued", workType: "story" }],
      name: "document-only",
      outputs: [{ state: "done", workType: "story" }],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

it("undoes and redoes mixed document transactions in strict LIFO order", () => {
  const { result } = renderEditableFactoryGraphHook({
    currentFactoryDocument: documentFactory,
  });

  act(() => {
    result.current.actions.addNode({
      capacity: "2",
      kind: "resource",
      name: "cache",
    });
  });
  act(() => {
    result.current.actions.updateNodeField({
      field: "body",
      kind: "workstation",
      name: "document-only",
      value: "Updated document instructions.",
    });
  });
  act(() => {
    result.current.actions.moveLayoutNode("workstation:document-only", {
      x: 120,
      y: 80,
    });
  });

  expect(result.current.pendingState.canUndoLayout).toBe(true);
  expect(result.current.saveState.canSave).toBe(true);
  expect(result.current.pendingState.pendingFactoryDefinition).toMatchObject({
    resources: [{ capacity: 2, name: "cache" }],
    workstations: [
      { body: "Updated document instructions.", name: "document-only" },
    ],
  });
  expect(layoutPosition(result)).toEqual({ x: 120, y: 80 });

  act(() => {
    result.current.actions.undoLayout();
  });
  expect(layoutPosition(result)).toBeUndefined();
  expect(
    result.current.pendingState.pendingFactoryDefinition?.workstations?.[0]
      ?.body,
  ).toBe("Updated document instructions.");

  act(() => {
    result.current.actions.undoLayout();
    result.current.actions.undoLayout();
  });
  expect(result.current.pendingState.pendingFactoryDefinition).toMatchObject({
    resources: [],
    workstations: [{ body: "Document plane baseline.", name: "document-only" }],
  });
  expect(result.current.pendingState.hasChanges).toBe(false);
  expect(result.current.saveState.canSave).toBe(false);

  act(() => {
    result.current.actions.redoLayout();
    result.current.actions.redoLayout();
    result.current.actions.redoLayout();
  });
  expect(result.current.pendingState.pendingFactoryDefinition).toMatchObject({
    resources: [{ capacity: 2, name: "cache" }],
    workstations: [
      { body: "Updated document instructions.", name: "document-only" },
    ],
  });
  expect(layoutPosition(result)).toEqual({ x: 120, y: 80 });
  expect(result.current.saveState.canSave).toBe(true);
});

function layoutPosition(
  result: ReturnType<typeof renderEditableFactoryGraphHook>["result"],
) {
  return result.current.layoutDraftState.layout.nodes?.find(
    (node) => node.id === "workstation:document-only",
  )?.position;
}

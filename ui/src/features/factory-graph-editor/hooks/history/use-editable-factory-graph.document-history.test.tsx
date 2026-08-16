// @component-test-runner vitest: imports workspace graph packages that Bun resolves through declaration files.
import { act, renderHook, waitFor } from "@testing-library/react";

import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import {
  createEditableFactoryGraphHookWrapper,
  renderEditableFactoryGraphHook,
  setupEditableFactoryGraphSaveTestEnvironment,
} from "../../../../testing/editable-factory-graph-hook-test-helpers";
import { mockFactoryDocumentSave } from "../../../../testing/factory-document-save-mocks";
import { createEmptyFactoryGraphDraft } from "../../lib/draft/factory-graph-draft-types";
import { useEditableFactoryGraph } from "../use-editable-factory-graph";

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

const parityFactory: CurrentFactoryDocument = {
  ...documentFactory,
  name: "Parity Factory",
  layout: {
    edges: [
      {
        id: "workstation-output:workstation:document-only->work-state:story:done",
        waypoints: [{ x: 180, y: 220 }],
      },
    ],
    groups: [
      {
        bounds: { height: 300, width: 420, x: 10, y: 20 },
        id: "group-1",
        label: "Baseline group",
        nodeIds: [],
      },
    ],
    nodes: [
      {
        id: "workstation:document-only",
        position: { x: 40, y: 60 },
      },
    ],
    schemaVersion: 1,
    viewport: { x: 4, y: 8, zoom: 1 },
  },
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

it("keeps layout, waypoint, viewport, and visual-group edits in document LIFO history", () => {
  const { result } = renderEditableFactoryGraphHook({
    currentFactoryDocument: parityFactory,
  });
  const snapshots = [captureDocumentSnapshot(result)];
  const commit = (operation: () => void) => {
    act(operation);
    snapshots.push(captureDocumentSnapshot(result));
  };

  commit(() => {
    result.current.actions.addNode({
      capacity: "2",
      kind: "resource",
      name: "cache",
    });
  });
  commit(() => {
    result.current.actions.updateNodeField({
      field: "body",
      kind: "workstation",
      name: "document-only",
      value: "Updated parity instructions.",
    });
  });
  commit(() => {
    result.current.actions.moveLayoutNode("workstation:document-only", {
      x: 120,
      y: 160,
    });
  });
  commit(() => {
    result.current.actions.resizeLayoutNode(
      "workstation:document-only",
      "workstation",
      { height: 400, width: 9999 },
      { x: 120, y: 160 },
    );
  });
  commit(() => {
    result.current.actions.updateLayoutViewport({
      x: 48,
      y: 96,
      zoom: 1.25,
    });
  });
  commit(() => {
    result.current.actions.addEdgeWaypoint(
      "workstation-output:workstation:document-only->work-state:story:done",
      { x: 240, y: 280 },
    );
  });
  commit(() => {
    result.current.actions.renameVisualGroup("group-1", "Renamed group");
  });
  commit(() => {
    result.current.actions.addNodeToVisualGroup(
      "group-1",
      "workstation:document-only",
    );
  });
  commit(() => {
    result.current.actions.moveVisualGroupByDelta(
      "group-1",
      { x: 25, y: 35 },
      new Map([["workstation:document-only", { x: 120, y: 160 }]]),
    );
  });
  commit(() => {
    result.current.actions.resizeVisualGroup("group-1", {
      height: 360,
      width: 500,
      x: 35,
      y: 55,
    });
  });

  expect(result.current.pendingState.canUndoLayout).toBe(true);
  expect(result.current.pendingState.hasChanges).toBe(true);

  for (let index = snapshots.length - 1; index > 0; index -= 1) {
    act(() => {
      result.current.actions.undoLayout();
    });
    expect(captureDocumentSnapshot(result)).toEqual(snapshots[index - 1]);
  }

  expect(result.current.pendingState.canUndoLayout).toBe(false);
  expect(result.current.pendingState.canRedoLayout).toBe(true);

  for (let index = 1; index < snapshots.length; index += 1) {
    act(() => {
      result.current.actions.redoLayout();
    });
    expect(captureDocumentSnapshot(result)).toEqual(snapshots[index]);
  }
  expect(result.current.pendingState.canRedoLayout).toBe(false);
});

it("clears unified history when discarding a mixed document", () => {
  const { result } = renderEditableFactoryGraphHook({
    currentFactoryDocument: parityFactory,
  });

  act(() => {
    result.current.actions.addNode({
      capacity: "2",
      kind: "resource",
      name: "discarded-cache",
    });
    result.current.actions.moveLayoutNode("workstation:document-only", {
      x: 500,
      y: 600,
    });
  });

  act(() => {
    result.current.actions.discard();
  });

  expect(captureDocumentSnapshot(result)).toEqual({
    draft: createEmptyFactoryGraphDraft(),
    layout: parityFactory.layout,
  });
  expect(result.current.pendingState.hasChanges).toBe(false);
  expect(result.current.pendingState.canUndoLayout).toBe(false);
  expect(result.current.pendingState.canRedoLayout).toBe(false);
});

it("rebases unified history after save and does not resurrect it on reload or scope replacement", async () => {
  setupEditableFactoryGraphSaveTestEnvironment(
    mockFactoryDocumentSave({ mode: "success" }),
  );
  const wrapper =
    createEditableFactoryGraphHookWrapper().EditableFactoryGraphHookWrapper;
  const replacementFactory: CurrentFactoryDocument = {
    ...parityFactory,
    name: "Replacement Factory",
    layout: {
      ...parityFactory.layout,
      viewport: { x: -100, y: -50, zoom: 2 },
    },
    version: {
      logical: "10",
      physical: "2026-06-01T01:00:00Z",
    },
  };
  const { result, rerender } = renderHook(
    ({
      currentFactoryDocument,
      factoryDocumentScopeKey,
    }: {
      currentFactoryDocument: CurrentFactoryDocument;
      factoryDocumentScopeKey: string;
    }) =>
      // The hook is rendered directly here so reload and scope replacement
      // can be observed through the same document-history instance.
      useEditableFactoryGraph({
        currentFactoryDocument,
        factoryDocumentScopeKey,
      }),
    {
      initialProps: {
        currentFactoryDocument: parityFactory,
        factoryDocumentScopeKey: "parity-session",
      },
      wrapper,
    },
  );

  act(() => {
    result.current.actions.addNode({
      capacity: "2",
      kind: "resource",
      name: "saved-cache",
    });
  });
  expect(result.current.pendingState.canUndoLayout).toBe(true);

  await act(async () => {
    await result.current.actions.save();
  });
  expect(result.current.pendingState.hasChanges).toBe(false);
  expect(result.current.pendingState.canUndoLayout).toBe(false);
  expect(result.current.pendingState.canRedoLayout).toBe(false);

  rerender({
    currentFactoryDocument: {
      ...parityFactory,
      layout: {
        ...parityFactory.layout,
        viewport: { x: 12, y: 24, zoom: 1.5 },
      },
      version: {
        logical: "11",
        physical: "2026-06-01T02:00:00Z",
      },
    },
    factoryDocumentScopeKey: "parity-session",
  });
  await waitFor(() => {
    expect(result.current.layoutDraftState.layout.viewport).toEqual({
      x: 12,
      y: 24,
      zoom: 1.5,
    });
  });
  expect(result.current.pendingState.canUndoLayout).toBe(false);
  expect(result.current.pendingState.canRedoLayout).toBe(false);

  act(() => {
    result.current.actions.moveLayoutNode("workstation:document-only", {
      x: 700,
      y: 800,
    });
  });
  rerender({
    currentFactoryDocument: replacementFactory,
    factoryDocumentScopeKey: "replacement-session",
  });
  await waitFor(() => {
    expect(result.current.draftState.latestDocument?.name).toBe(
      "Replacement Factory",
    );
  });
  expect(result.current.pendingState.hasChanges).toBe(false);
  expect(result.current.pendingState.canUndoLayout).toBe(false);
  expect(result.current.pendingState.canRedoLayout).toBe(false);
  expect(result.current.layoutDraftState.layout.viewport).toEqual(
    replacementFactory.layout?.viewport,
  );
});

function captureDocumentSnapshot(
  result: ReturnType<typeof renderEditableFactoryGraphHook>["result"],
) {
  return {
    draft: structuredClone(result.current.draftState.draft),
    layout: structuredClone(result.current.layoutDraftState.layout),
  };
}

function layoutPosition(
  result: ReturnType<typeof renderEditableFactoryGraphHook>["result"],
) {
  return result.current.layoutDraftState.layout.nodes?.find(
    (node) => node.id === "workstation:document-only",
  )?.position;
}

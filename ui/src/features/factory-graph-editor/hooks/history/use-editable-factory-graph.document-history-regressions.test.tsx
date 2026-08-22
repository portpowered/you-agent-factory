// @component-test-runner vitest: imports workspace graph packages that Bun resolves through declaration files.
import { act } from "@testing-library/react";

import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { renderEditableFactoryGraphHook } from "../../../../testing/editable-factory-graph-hook-test-helpers";

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

it("does not add a history entry for a blocked operation", () => {
  const { result } = renderEditableFactoryGraphHook({
    currentFactoryDocument: documentFactory,
  });
  const initialSnapshot = captureDocumentSnapshot(result);

  act(() => {
    result.current.actions.addNode({
      capacity: "2",
      kind: "resource",
      name: "cache",
    });
  });
  const successfulSnapshot = captureDocumentSnapshot(result);

  let blockedResult: unknown;
  act(() => {
    blockedResult = result.current.actions.removeNode("worker:writer");
  });

  expect(blockedResult).toMatchObject({
    ok: false,
    reason: "BLOCKED_REMOVAL",
  });
  expect(captureDocumentSnapshot(result)).toEqual(successfulSnapshot);
  expect(result.current.pendingState.canUndoLayout).toBe(true);

  act(() => {
    result.current.actions.undoLayout();
  });
  expect(captureDocumentSnapshot(result)).toEqual(initialSnapshot);

  act(() => {
    result.current.actions.redoLayout();
  });
  expect(captureDocumentSnapshot(result)).toEqual(successfulSnapshot);
});

it("undoes and redoes node deletion without orphaning its layout projection", () => {
  const deletionFactory: CurrentFactoryDocument = {
    ...parityFactory,
    resources: [{ capacity: 2, name: "gpu" }],
    layout: {
      ...parityFactory.layout,
      nodes: [
        {
          id: "resource:gpu",
          position: { x: 320, y: 240 },
        },
        ...(parityFactory.layout?.nodes ?? []),
      ],
    },
  };
  const { result } = renderEditableFactoryGraphHook({
    currentFactoryDocument: deletionFactory,
  });
  const initialSnapshot = captureDocumentSnapshot(result);
  const initialResourcePosition = result.current.projection.nodes.find(
    (node) => node.id === "resource:gpu",
  )?.position;

  let removalResult: unknown;
  act(() => {
    removalResult = result.current.actions.removeNode("resource:gpu");
  });

  expect(removalResult).toMatchObject({ ok: true });
  expect(
    result.current.projection.nodes.some((node) => node.id === "resource:gpu"),
  ).toBe(false);
  expect(
    result.current.projection.edges.some(
      (edge) =>
        edge.source === "resource:gpu" || edge.target === "resource:gpu",
    ),
  ).toBe(false);

  act(() => {
    result.current.actions.undoLayout();
  });
  expect(captureDocumentSnapshot(result)).toEqual(initialSnapshot);
  expect(
    result.current.projection.nodes.find((node) => node.id === "resource:gpu")
      ?.position,
  ).toEqual(initialResourcePosition);

  act(() => {
    result.current.actions.redoLayout();
  });
  expect(
    result.current.projection.nodes.some((node) => node.id === "resource:gpu"),
  ).toBe(false);
  expect(
    result.current.projection.edges.some(
      (edge) =>
        edge.source === "resource:gpu" || edge.target === "resource:gpu",
    ),
  ).toBe(false);
});

function captureDocumentSnapshot(
  result: ReturnType<typeof renderEditableFactoryGraphHook>["result"],
) {
  return {
    draft: structuredClone(result.current.draftState.draft),
    layout: structuredClone(result.current.layoutDraftState.layout),
  };
}

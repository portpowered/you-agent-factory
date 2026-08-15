import { act, renderHook } from "@testing-library/react";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import {
  createEditableFactoryGraphHookWrapper,
  renderEditableFactoryGraphHook,
  setupEditableFactoryGraphSaveTestEnvironment,
} from "../../../testing/editable-factory-graph-hook-test-helpers";
import { mockFactoryDocumentSave } from "../../../testing/factory-document-save-mocks";
import { useEditableFactoryGraph } from "./use-editable-factory-graph";

const modelWorkerAddOperations = [
  {
    name: "REVIEW",
    inputs: [{ name: "text", contentTypes: ["TEXT"], required: true }],
    outputs: [{ name: "result", contentTypes: ["TEXT"] }],
  },
] as const;

const sharedWorkType = {
  name: "story",
  states: [
    {
      name: "queued",
      type: "INITIAL" as const,
    },
    {
      name: "done",
      type: "TERMINAL" as const,
    },
  ],
};

const documentFactory: CurrentFactoryDocument = {
  name: "Document Factory",
  version: {
    logical: "9",
    physical: "2026-05-31T01:00:00Z",
  },
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [sharedWorkType],
  workstations: [
    {
      body: "Document plane baseline.",
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "document-only",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

describe("useEditableFactoryGraph document plane projection", () => {
  beforeEach(() => {
    setupEditableFactoryGraphSaveTestEnvironment();
  });

  it("projects editable graph nodes from the loaded factory document only", () => {
    const { result } = renderEditableFactoryGraphHook({
      currentFactoryDocument: documentFactory,
    });

    expect(result.current.draftState.source).toBe("current-factory");
    expect(result.current.draftState.baseDocument).toEqual(documentFactory);
    expect(result.current.draftState.latestDocument).toEqual(documentFactory);
    expect(result.current.projection.nodes.map((node) => node.id)).toContain(
      "workstation:document-only",
    );
    expect(
      result.current.projection.nodes.map((node) => node.id),
    ).not.toContain("workstation:snapshot-only");
  });

  it("exposes an empty projection while the factory document is still unavailable", () => {
    const { result } = renderEditableFactoryGraphHook({});

    expect(result.current.draftState.source).toBe("projection");
    expect(result.current.draftState.baseDocument).toBeNull();
    expect(result.current.draftState.latestDocument).toBeNull();
    expect(result.current.projection).toEqual({
      edges: [],
      nodes: [],
    });
    expect(result.current.graphState).toBeNull();
  });
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: document-plane scope scenarios share one fixture setup.
describe("useEditableFactoryGraph document plane scope isolation", () => {
  beforeEach(() => {
    setupEditableFactoryGraphSaveTestEnvironment();
  });

  it("does not rehydrate from a stale document while the new scope document is pending", () => {
    const betaFactory: CurrentFactoryDocument = {
      ...documentFactory,
      name: "Beta Factory Pending",
      workstations: [
        {
          ...documentFactory.workstations[0],
          name: "beta-pending-only",
        },
      ],
    };
    const { result, rerender } = renderHook(
      ({
        currentFactoryDocument,
        factoryDocumentScopeKey,
      }: {
        currentFactoryDocument: CurrentFactoryDocument | undefined;
        factoryDocumentScopeKey: string;
      }) =>
        useEditableFactoryGraph({
          currentFactoryDocument,
          factoryDocumentScopeKey,
        }),
      {
        initialProps: {
          currentFactoryDocument: documentFactory,
          factoryDocumentScopeKey: "session-alpha",
        },
        wrapper:
          createEditableFactoryGraphHookWrapper()
            .EditableFactoryGraphHookWrapper,
      },
    );

    act(() => {
      result.current.actions.addNode({
        argsText: "",
        command: "",
        kind: "worker",
        model: "gpt-5",
        modelProvider: "CODEX",
        name: "extra",
        operations: modelWorkerAddOperations,
        workerType: "MODEL_WORKER",
      });
    });

    rerender({
      currentFactoryDocument: documentFactory,
      factoryDocumentScopeKey: "session-beta",
    });

    expect(result.current.pendingState.hasChanges).toBe(false);
    expect(result.current.draftState.source).toBe("projection");
    expect(result.current.draftState.latestDocument).toBeNull();
    expect(result.current.projection).toEqual({
      edges: [],
      nodes: [],
    });

    rerender({
      currentFactoryDocument: betaFactory,
      factoryDocumentScopeKey: "session-beta",
    });

    expect(result.current.pendingState.hasChanges).toBe(false);
    expect(result.current.draftState.latestDocument?.name).toBe(
      "Beta Factory Pending",
    );
    expect(result.current.projection.nodes.map((node) => node.id)).toContain(
      "workstation:beta-pending-only",
    );
    expect(
      result.current.projection.nodes.map((node) => node.id),
    ).not.toContain("workstation:document-only");
  });

  it("drops a dirty draft when the factory document scope key changes", () => {
    const { result, rerender } = renderHook(
      ({
        currentFactoryDocument,
        factoryDocumentScopeKey,
      }: {
        currentFactoryDocument: CurrentFactoryDocument;
        factoryDocumentScopeKey: string;
      }) =>
        useEditableFactoryGraph({
          currentFactoryDocument,
          factoryDocumentScopeKey,
        }),
      {
        initialProps: {
          currentFactoryDocument: documentFactory,
          factoryDocumentScopeKey: "session-alpha",
        },
        wrapper:
          createEditableFactoryGraphHookWrapper()
            .EditableFactoryGraphHookWrapper,
      },
    );

    act(() => {
      result.current.actions.addNode({
        argsText: "",
        command: "",
        kind: "worker",
        model: "gpt-5",
        modelProvider: "CODEX",
        name: "extra",
        operations: modelWorkerAddOperations,
        workerType: "MODEL_WORKER",
      });
    });

    expect(result.current.pendingState.hasChanges).toBe(true);

    rerender({
      currentFactoryDocument: {
        ...documentFactory,
        name: "Beta Factory",
        workstations: [
          {
            ...documentFactory.workstations[0],
            name: "beta-only",
          },
        ],
      },
      factoryDocumentScopeKey: "session-beta",
    });

    expect(result.current.pendingState.hasChanges).toBe(false);
    expect(result.current.draftState.latestDocument?.name).toBe("Beta Factory");
    expect(result.current.projection.nodes.map((node) => node.id)).toContain(
      "workstation:beta-only",
    );
    expect(
      result.current.projection.nodes.map((node) => node.id),
    ).not.toContain("workstation:document-only");
  });
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: document-plane save regressions share one hook harness.
describe("useEditableFactoryGraph document plane persist", () => {
  beforeEach(() => {
    setupEditableFactoryGraphSaveTestEnvironment();
  });

  it("keeps deleted factory-change resource nodes removed immediately after save", async () => {
    const resourceFactory: CurrentFactoryDocument = {
      name: "Factory Stream Delete",
      resources: [
        {
          capacity: 1,
          name: "rge",
        },
        {
          capacity: 1,
          name: "asdasd",
        },
      ],
      version: {
        logical: "9",
        physical: "2026-06-10T10:37:07.833698Z",
      },
    };
    const saveMutation = setupEditableFactoryGraphSaveTestEnvironment(
      mockFactoryDocumentSave({
        mode: "success",
        resolvedDocument: {
          ...resourceFactory,
          resources: [
            {
              capacity: 1,
              name: "asdasd",
            },
          ],
          version: {
            logical: "10",
            physical: "2026-06-10T10:37:39.734365Z",
          },
        },
      }),
    );
    const { result } = renderHook(
      () =>
        useEditableFactoryGraph({
          currentFactoryDocument: resourceFactory,
          factoryDocumentScopeKey: "session-alpha",
        }),
      {
        wrapper:
          createEditableFactoryGraphHookWrapper()
            .EditableFactoryGraphHookWrapper,
      },
    );

    expect(result.current.projection.nodes.map((node) => node.id)).toEqual(
      expect.arrayContaining(["resource:asdasd", "resource:rge"]),
    );

    act(() => {
      result.current.actions.removeNode("resource:rge");
    });

    expect(result.current.pendingState.hasTopologyChanges).toBe(true);
    expect(
      result.current.projection.nodes.map((node) => node.id),
    ).not.toContain("resource:rge");

    await act(async () => {
      await result.current.actions.save();
    });

    expect(saveMutation.saveAsync).toHaveBeenCalledWith({
      baseVersion: resourceFactory.version,
      factory: expect.objectContaining({
        resources: [
          expect.objectContaining({
            capacity: 1,
            name: "asdasd",
          }),
        ],
      }),
    });
    expect(result.current.pendingState.hasTopologyChanges).toBe(false);
    expect(result.current.draftState.latestDocument?.version.logical).toBe(
      "10",
    );
    expect(result.current.projection.nodes.map((node) => node.id)).toEqual([
      "resource:asdasd",
    ]);
  });

  it("clears pending edits after save and resyncs latestDocument when the document cache updates", async () => {
    const saveMutation = setupEditableFactoryGraphSaveTestEnvironment(
      mockFactoryDocumentSave({ mode: "success" }),
    );

    const { result, rerender } = renderHook(
      ({
        currentFactoryDocument,
      }: {
        currentFactoryDocument: CurrentFactoryDocument;
      }) =>
        useEditableFactoryGraph({
          currentFactoryDocument,
          factoryDocumentScopeKey: "session-alpha",
        }),
      {
        initialProps: {
          currentFactoryDocument: documentFactory,
        },
        wrapper:
          createEditableFactoryGraphHookWrapper()
            .EditableFactoryGraphHookWrapper,
      },
    );

    act(() => {
      result.current.actions.addNode({
        argsText: "",
        command: "",
        kind: "worker",
        model: "gpt-5",
        modelProvider: "CODEX",
        name: "extra",
        operations: modelWorkerAddOperations,
        workerType: "MODEL_WORKER",
      });
    });

    expect(result.current.pendingState.hasChanges).toBe(true);

    await act(async () => {
      await result.current.actions.save();
    });

    expect(saveMutation.saveAsync).toHaveBeenCalledWith({
      baseVersion: documentFactory.version,
      factory: expect.objectContaining({
        workers: expect.arrayContaining([
          expect.objectContaining({ name: "extra" }),
        ]),
      }),
    });
    expect(result.current.pendingState.hasChanges).toBe(false);

    const savedDocument: CurrentFactoryDocument = {
      ...documentFactory,
      version: {
        logical: "10",
        physical: "2026-05-31T02:00:00Z",
      },
    };

    rerender({ currentFactoryDocument: savedDocument });

    expect(result.current.draftState.latestDocument).toEqual(savedDocument);
    expect(result.current.draftState.baseDocument?.version.logical).toBe("10");
    expect(result.current.pendingState.hasChanges).toBe(false);
  });
});

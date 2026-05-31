import { act, renderHook } from "@testing-library/react";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import { mockFactoryDocumentSave } from "../../../testing/factory-document-save-mocks";
import {
  createEditableFactoryGraphHookWrapper,
  renderEditableFactoryGraphHook,
  setupEditableFactoryGraphSaveTestEnvironment,
} from "../../../testing/editable-factory-graph-hook-test-helpers";
import { useEditableFactoryGraph } from "./use-editable-factory-graph";

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

describe("useEditableFactoryGraph document plane", () => {
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
    expect(result.current.projection.nodes.map((node) => node.id)).not.toContain(
      "workstation:snapshot-only",
    );
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
        wrapper: createEditableFactoryGraphHookWrapper(),
      },
    );

    act(() => {
      result.current.actions.addNode({
        kind: "worker",
        model: "gpt-5",
        modelProvider: "CURSOR",
        name: "extra",
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
    expect(result.current.projection.nodes.map((node) => node.id)).not.toContain(
      "workstation:document-only",
    );
  });

  it("clears pending edits after save and resyncs latestDocument when the document cache updates", async () => {
    const saveMutation = setupEditableFactoryGraphSaveTestEnvironment(
      mockFactoryDocumentSave({ mode: "success" }),
    );

    const { result, rerender } = renderHook(
      ({ currentFactoryDocument }: { currentFactoryDocument: CurrentFactoryDocument }) =>
        useEditableFactoryGraph({
          currentFactoryDocument,
          factoryDocumentScopeKey: "session-alpha",
        }),
      {
        initialProps: {
          currentFactoryDocument: documentFactory,
        },
        wrapper: createEditableFactoryGraphHookWrapper(),
      },
    );

    act(() => {
      result.current.actions.addNode({
        kind: "worker",
        model: "gpt-5",
        modelProvider: "CURSOR",
        name: "extra",
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

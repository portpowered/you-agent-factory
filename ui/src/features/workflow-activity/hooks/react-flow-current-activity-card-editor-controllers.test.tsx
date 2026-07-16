// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: existing current-activity editor-controller coverage stayed intact during feature-root migration.
// biome-ignore lint/style/noExcessiveLinesPerFile: controller coverage grew with topology delete regressions; split is deferred to keep one harness seam.
import { act, renderHook } from "@testing-library/react";

import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import { buildFactoryGraphTopologyFromDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft-graph";
import type {
  FactoryGraphDraft,
  FactoryGraphTopology,
} from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import { createEmptyFactoryGraphDraft } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";
import { useFactoryGraphRemovalController } from "./react-flow-current-activity-card-editor-removals";

const baseFactoryDefinition: CanonicalFactoryDefinition = {
  name: "Current Factory",
  resources: [
    {
      capacity: 2,
      name: "gpu",
    },
  ],
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
        {
          name: "done",
          type: "TERMINAL",
        },
      ],
    },
  ],
  workstations: [
    {
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "review",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      resources: [
        {
          capacity: 2,
          name: "gpu",
        },
      ],
      stopWords: ["DONE"],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

const routingTransitionFactoryDefinition: CanonicalFactoryDefinition = {
  ...baseFactoryDefinition,
  workTypes: [
    {
      name: "story",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
        {
          name: "done",
          type: "TERMINAL",
        },
        {
          name: "failed",
          type: "TERMINAL",
        },
      ],
    },
  ],
  workstations: [
    {
      ...baseFactoryDefinition.workstations?.[0],
      onContinue: [
        {
          state: "done",
          workType: "story",
        },
      ],
      onFailure: [
        {
          state: "failed",
          workType: "story",
        },
      ],
      onRejection: [
        {
          state: "queued",
          workType: "story",
        },
      ],
    },
  ],
};

const editableDocument = {
  ...baseFactoryDefinition,
  version: {
    logical: "4",
    physical: "2026-05-20T03:45:00Z",
  },
};

describe("current activity graph editor controllers", () => {
  it.each([
    {
      edgeKind: "workstation-output",
      sourceAnchorId: "workstation-output-source",
      targetAnchorId: "work-state-input-target",
      targetNodeId: "work-state:story:done",
      targetStateName: "done",
    },
    {
      edgeKind: "workstation-on-continue",
      sourceAnchorId: "workstation-on-continue-source",
      targetAnchorId: "work-state-input-target",
      targetNodeId: "work-state:story:queued",
      targetStateName: "queued",
    },
    {
      edgeKind: "workstation-on-failure",
      sourceAnchorId: "workstation-on-failure-source",
      targetAnchorId: "work-state-input-target",
      targetNodeId: "work-state:story:queued",
      targetStateName: "queued",
    },
    {
      edgeKind: "workstation-on-rejection",
      sourceAnchorId: "workstation-on-rejection-source",
      targetAnchorId: "work-state-input-target",
      targetNodeId: "work-state:story:done",
      targetStateName: "done",
    },
  ])("commits %s keyboard anchor connections and clears the pending source", ({
    edgeKind,
    sourceAnchorId,
    targetAnchorId,
    targetNodeId,
  }) => {
    const graph = createDraftState().graph;
    const draftState = createDraftState({
      graph: {
        ...graph,
        edges: graph.edges.filter((edge) => edge.kind !== edgeKind),
      },
    });
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphConnectionController({
        activeTool: "connect",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
      }),
    );

    act(() => {
      result.current.handleConnectionAnchorClick({
        anchorId: sourceAnchorId,
        nodeId: "workstation:review",
      });
    });

    expect(result.current.pendingConnectionSource).toEqual({
      anchorId: sourceAnchorId,
      nodeId: "workstation:review",
    });

    act(() => {
      result.current.handleConnectionAnchorClick({
        anchorId: targetAnchorId,
        nodeId: targetNodeId,
      });
    });

    expect(editableGraph.actions.connectNodes).toHaveBeenCalledWith({
      sourceAnchorId,
      sourceNodeId: "workstation:review",
      targetAnchorId,
      targetNodeId,
    });
    expect(result.current.connectionNotice).toBeNull();
    expect(result.current.pendingConnectionSource).toBeNull();
  });

  it("shows actionable connection notices for invalid connection paths", () => {
    const draftState = createDraftState();
    const editableGraph = createEditableGraph({
      connectNodes: vi.fn(() => ({
        message:
          "Failure connections from review cannot connect to Continue on story:done.",
        ok: false,
        reason: "INVALID_CONNECTION",
      })),
    });

    const { result } = renderHook(() =>
      useFactoryGraphConnectionController({
        activeTool: "connect",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
      }),
    );

    act(() => {
      result.current.handleConnectionAnchorClick({
        anchorId: "work-state-input-target",
        nodeId: "work-state:story:done",
      });
    });

    expect(result.current.connectionNotice).toBe(
      "Select a source anchor before choosing a target anchor.",
    );

    act(() => {
      result.current.handleEditorConnect({
        source: "workstation:review",
        sourceHandle: "workstation-on-failure-source",
        target: "work-state:story:done",
        targetHandle: "work-state-input-target",
      });
    });

    expect(result.current.connectionNotice).toBe(
      "Failure connections from review cannot connect to Continue on story:done.",
    );
    expect(editableGraph.actions.connectNodes).toHaveBeenCalledTimes(1);
  });

  it("ignores continue and reject anchor clicks for standard processors without stopWords", () => {
    const factoryWithoutStopWords: CanonicalFactoryDefinition = {
      ...baseFactoryDefinition,
      workstations: [
        {
          ...baseFactoryDefinition.workstations?.[0],
          stopWords: undefined,
        },
      ],
    };
    const draftState = createDraftState({
      baseFactoryDefinition: factoryWithoutStopWords,
      graph: buildFactoryGraphTopologyFromDefinition(factoryWithoutStopWords),
    });
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphConnectionController({
        activeTool: "connect",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
      }),
    );

    act(() => {
      result.current.handleConnectionAnchorClick({
        anchorId: "workstation-on-continue-source",
        nodeId: "workstation:review",
      });
    });

    expect(result.current.pendingConnectionSource).toBeNull();
    expect(editableGraph.actions.connectNodes).not.toHaveBeenCalled();
  });

  it.each([
    {
      edgeId: "workstation-output:workstation:review->work-state:story:done",
    },
    {
      edgeId:
        "workstation-on-continue:workstation:review->work-state:story:done",
      factoryDefinition: routingTransitionFactoryDefinition,
    },
    {
      edgeId:
        "workstation-on-failure:workstation:review->work-state:story:failed",
      factoryDefinition: routingTransitionFactoryDefinition,
    },
    {
      edgeId:
        "workstation-on-rejection:workstation:review->work-state:story:queued",
      factoryDefinition: routingTransitionFactoryDefinition,
    },
    {
      edgeId: "workstation-resource:resource:gpu->workstation:review",
    },
    {
      edgeId: "worker-assignment:worker:writer->workstation:review",
    },
  ])("opens confirmation before applying removable $edgeId draft edge removals", ({
    edgeId,
    factoryDefinition = baseFactoryDefinition,
  }) => {
    const reset = vi.fn();
    const draftState = createDraftState({
      baseFactoryDefinition: factoryDefinition,
    });
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorEdgeDelete(edgeId);
    });

    expect(editableGraph.actions.disconnectEdge).not.toHaveBeenCalled();
    expect(result.current.pendingRemovalIntent).toMatchObject({
      requiresConfirmation: true,
    });

    act(() => {
      result.current.handleConfirmRemoval();
    });

    expect(reset).toHaveBeenCalledTimes(1);
    expect(editableGraph.actions.disconnectEdge).toHaveBeenCalledWith(edgeId);
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it("surfaces blocked work-type-state edge removal without mutating the draft", () => {
    const reset = vi.fn();
    const initialDraft = createEmptyFactoryGraphDraft();
    const draftState = createDraftState({ draft: initialDraft });
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorEdgeDelete(
        "work-type-state:work-type:story->work-state:story:queued",
      );
    });

    expect(reset).toHaveBeenCalledTimes(1);
    expect(editableGraph.actions.disconnectEdge).not.toHaveBeenCalled();
    expect(draftState.replaceDraft).not.toHaveBeenCalled();
    expect(draftState.draft).toBe(initialDraft);
    expect(result.current.blockedRemovalReason).toBe(
      "Work type ordering edges are managed by work-state membership and cannot be removed directly.",
    );
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it("ignores delete-tool edge clicks when activeTool is not delete", () => {
    const reset = vi.fn();
    const draftState = createDraftState();
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "connect",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorEdgeDelete(
        "workstation-output:workstation:review->work-state:story:done",
      );
    });

    expect(editableGraph.actions.disconnectEdge).not.toHaveBeenCalled();
    expect(reset).not.toHaveBeenCalled();
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it("surfaces blocked node-removal reasons without mutating the draft", () => {
    const reset = vi.fn();
    const initialDraft = createEmptyFactoryGraphDraft();
    const draftState = createDraftState({ draft: initialDraft });
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorNodeDelete("worker:writer");
    });

    expect(reset).toHaveBeenCalledTimes(1);
    expect(editableGraph.actions.removeSelection).not.toHaveBeenCalled();
    expect(draftState.replaceDraft).not.toHaveBeenCalled();
    expect(draftState.draft).toBe(initialDraft);
    expect(result.current.blockedRemovalReason).toBe(
      "This worker is still assigned to 1 workstation. Reassign or remove those workstations before deleting writer.",
    );
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it("surfaces blocked node-removal from removeNode without mutating the draft", () => {
    const reset = vi.fn();
    const initialDraft = createEmptyFactoryGraphDraft();
    const unassignedDefinition: CanonicalFactoryDefinition = {
      ...baseFactoryDefinition,
      workers: [
        ...(baseFactoryDefinition.workers ?? []),
        {
          model: "gpt-5",
          name: "editor",
          type: "MODEL_WORKER",
        },
      ],
    };
    const draftState = createDraftState({
      baseFactoryDefinition: unassignedDefinition,
      draft: initialDraft,
    });
    const editableGraph = createEditableGraph({
      removeSelection: vi.fn(() => ({
        message: "Removal blocked by graph operation layer.",
        ok: false,
        reason: "BLOCKED_REMOVAL",
      })),
    });

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorNodeDelete("worker:editor");
    });

    expect(editableGraph.actions.removeSelection).toHaveBeenCalledWith({
      edgeIds: [],
      nodeIds: ["worker:editor"],
    });
    expect(draftState.replaceDraft).not.toHaveBeenCalled();
    expect(draftState.draft).toBe(initialDraft);
    expect(result.current.blockedRemovalReason).toBe(
      "Removal blocked by graph operation layer.",
    );
  });

  it("opens confirmation before deleting a work type with owned states", () => {
    const reset = vi.fn();
    const draftState = createDraftState();
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorNodeDelete("work-type:story");
    });

    expect(editableGraph.actions.removeSelection).not.toHaveBeenCalled();
    expect(result.current.pendingRemovalIntent).toMatchObject({
      confirmDescription: expect.stringContaining("2 work states"),
      requiresConfirmation: true,
      title: "Remove story work-type?",
    });

    act(() => {
      result.current.handleCancelRemoval();
    });

    expect(editableGraph.actions.removeSelection).not.toHaveBeenCalled();
    expect(draftState.replaceDraft).not.toHaveBeenCalled();
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it("opens confirmation before deleting a workstation with connected edges and applies removal on confirm", () => {
    const reset = vi.fn();
    const draftState = createDraftState();
    const editableGraph = createEditableGraph();
    const onNodeRemovedFromDraft = vi.fn();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        onNodeRemovedFromDraft,
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorNodeDelete("workstation:review");
    });

    expect(editableGraph.actions.removeSelection).not.toHaveBeenCalled();
    expect(result.current.pendingRemovalIntent).toMatchObject({
      confirmDescription: expect.stringContaining("graph edge"),
      requiresConfirmation: true,
      title: "Remove review workstation?",
    });

    act(() => {
      result.current.handleConfirmRemoval();
    });

    expect(reset).toHaveBeenCalledTimes(1);
    expect(editableGraph.actions.removeSelection).toHaveBeenCalledWith({
      edgeIds: [],
      nodeIds: ["workstation:review"],
    });
    expect(onNodeRemovedFromDraft).toHaveBeenCalledWith("workstation:review");
    expect(result.current.pendingRemovalIntent).toBeNull();
    expect(result.current.blockedRemovalReason).toBeNull();
  });

  it("opens confirmation before deleting a bundled doc node and applies removal on confirm", () => {
    const reset = vi.fn();
    const factoryWithDoc: CanonicalFactoryDefinition = {
      ...baseFactoryDefinition,
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Guide\n" },
            targetPath: "factory/docs/guide.md",
            type: "DOC",
          },
        ],
      },
    };
    const draftState = createDraftState({
      baseFactoryDefinition: factoryWithDoc,
    });
    const editableGraph = createEditableGraph();
    const onNodeRemovedFromDraft = vi.fn();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        onNodeRemovedFromDraft,
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorNodeDelete("doc:factory/docs/guide.md");
    });

    expect(editableGraph.actions.removeSelection).not.toHaveBeenCalled();
    expect(result.current.pendingRemovalIntent).toMatchObject({
      requiresConfirmation: true,
      targetPath: "factory/docs/guide.md",
      title: "Remove guide.md doc?",
    });

    act(() => {
      result.current.handleConfirmRemoval();
    });

    expect(editableGraph.actions.removeSelection).toHaveBeenCalledWith({
      edgeIds: [],
      nodeIds: ["doc:factory/docs/guide.md"],
    });
    expect(onNodeRemovedFromDraft).toHaveBeenCalledWith(
      "doc:factory/docs/guide.md",
    );
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it("applies work-type removal through removeNode after delete-tool confirmation", () => {
    const reset = vi.fn();
    const draftState = createDraftState();
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorNodeDelete("work-type:story");
    });

    act(() => {
      result.current.handleConfirmRemoval();
    });

    expect(reset).toHaveBeenCalledTimes(1);
    expect(editableGraph.actions.removeSelection).toHaveBeenCalledWith({
      edgeIds: [],
      nodeIds: ["work-type:story"],
    });
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it("clears blocked removal notice when switching away from delete tool", () => {
    const reset = vi.fn();
    const draftState = createDraftState();
    const editableGraph = createEditableGraph();

    const { result, rerender } = renderHook(
      ({ activeTool }: { activeTool: "connect" | "delete" }) =>
        useFactoryGraphRemovalController({
          activeTool,
          canInteractWithEditor: true,
          draftState,
          editableGraph,
          hiddenNodeClasses: new Set(),
          saveEditableDefinition: {
            reset,
          } as never,
        }),
      { initialProps: { activeTool: "delete" as const } },
    );

    act(() => {
      result.current.handleEditorNodeDelete("worker:writer");
    });

    expect(result.current.blockedRemovalReason).toBe(
      "This worker is still assigned to 1 workstation. Reassign or remove those workstations before deleting writer.",
    );

    rerender({ activeTool: "connect" });

    expect(result.current.blockedRemovalReason).toBeNull();
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it("clears blocked removal notice when canceling after a blocked delete attempt", () => {
    const reset = vi.fn();
    const draftState = createDraftState();
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorNodeDelete("worker:writer");
    });

    expect(result.current.blockedRemovalReason).toBe(
      "This worker is still assigned to 1 workstation. Reassign or remove those workstations before deleting writer.",
    );

    act(() => {
      result.current.handleCancelRemoval();
    });

    expect(result.current.blockedRemovalReason).toBeNull();
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it("clears pending removal intent when canceling a confirmation dialog", () => {
    const reset = vi.fn();
    const draftState = createDraftState();
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorNodeDelete("work-type:story");
    });

    expect(result.current.pendingRemovalIntent).not.toBeNull();

    act(() => {
      result.current.handleCancelRemoval();
    });

    expect(result.current.pendingRemovalIntent).toBeNull();
    expect(result.current.blockedRemovalReason).toBeNull();
  });

  it("ignores delete-tool node clicks when activeTool is not delete", () => {
    const reset = vi.fn();
    const draftState = createDraftState();
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "connect",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorNodeDelete("worker:editor");
    });

    expect(editableGraph.actions.removeSelection).not.toHaveBeenCalled();
    expect(reset).not.toHaveBeenCalled();
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it("applies selection-panel node removal without requiring delete tool mode", () => {
    const reset = vi.fn();
    const unassignedDefinition: CanonicalFactoryDefinition = {
      ...baseFactoryDefinition,
      workers: [
        ...(baseFactoryDefinition.workers ?? []),
        {
          model: "gpt-5",
          name: "editor",
          type: "MODEL_WORKER",
        },
      ],
    };
    const draftState = createDraftState({
      baseFactoryDefinition: unassignedDefinition,
    });
    const editableGraph = createEditableGraph();
    const onNodeRemovedFromDraft = vi.fn();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: null,
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        onNodeRemovedFromDraft,
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleSelectionNodeDelete("worker:editor");
    });

    expect(reset).toHaveBeenCalledTimes(1);
    expect(editableGraph.actions.removeSelection).toHaveBeenCalledWith({
      edgeIds: [],
      nodeIds: ["worker:editor"],
    });
    expect(onNodeRemovedFromDraft).toHaveBeenCalledWith("worker:editor");
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it.each([
    {
      nodeId: "resource:cache",
    },
    {
      nodeId: "worker:editor",
    },
  ])("immediately applies removable $nodeId draft node removals", ({
    nodeId,
  }) => {
    const reset = vi.fn();
    const unassignedDefinition: CanonicalFactoryDefinition = {
      ...baseFactoryDefinition,
      resources: [
        ...(baseFactoryDefinition.resources ?? []),
        { capacity: 1, name: "cache" },
      ],
      workers: [
        ...(baseFactoryDefinition.workers ?? []),
        {
          model: "gpt-5",
          name: "editor",
          type: "MODEL_WORKER",
        },
      ],
    };
    const draftState = createDraftState({
      baseFactoryDefinition: unassignedDefinition,
    });
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorNodeDelete(nodeId);
    });

    expect(reset).toHaveBeenCalledTimes(1);
    expect(editableGraph.actions.removeSelection).toHaveBeenCalledWith({
      edgeIds: [],
      nodeIds: [nodeId],
    });
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it("applies batch selection delete for multiple draft-only resources", () => {
    const reset = vi.fn();
    const draft = createEmptyFactoryGraphDraft();
    draft.additions.resources.push(
      { capacity: 1, name: "cache" },
      { capacity: 1, name: "queue" },
    );
    const draftState = createDraftState({ draft });
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: null,
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        hiddenNodeClasses: new Set(),
        saveEditableDefinition: {
          reset,
        } as never,
      }),
    );

    act(() => {
      result.current.handleSelectionBatchDelete({
        edgeIds: [],
        nodeIds: ["resource:cache", "resource:queue"],
      });
    });

    expect(editableGraph.actions.removeSelection).toHaveBeenCalledWith({
      edgeIds: [],
      nodeIds: ["resource:cache", "resource:queue"],
    });
    expect(reset).toHaveBeenCalledTimes(1);
    expect(result.current.pendingRemovalIntent).toBeNull();
  });
});

function createDraftState({
  baseFactoryDefinition: factoryDefinition = baseFactoryDefinition,
  draft = createEmptyFactoryGraphDraft(),
  graph = buildFactoryGraphTopologyFromDefinition(factoryDefinition),
  updateDraft = vi.fn(),
}: {
  baseFactoryDefinition?: CanonicalFactoryDefinition;
  draft?: FactoryGraphDraft;
  graph?: FactoryGraphTopology;
  updateDraft?: ReturnType<typeof vi.fn>;
} = {}) {
  const document = {
    ...factoryDefinition,
    version: editableDocument.version,
  };

  return {
    baseDocument: document,
    draft,
    graph,
    hasChanges: false,
    latestDocument: document,
    pendingFactoryDefinition: factoryDefinition,
    replaceDraft: vi.fn(),
    resetDraft: vi.fn(),
    source: "current-factory" as const,
    updateDraft,
    validationErrors: [],
  };
}

function createEditableGraph(
  actionOverrides: Partial<EditableFactoryGraphViewModel["actions"]> = {},
): EditableFactoryGraphViewModel {
  const emptyDraft = createEmptyFactoryGraphDraft();

  return {
    actions: {
      addNode: vi.fn(() => ({ ok: true, value: emptyDraft })),
      connectNodes: vi.fn(() => ({ ok: true, value: emptyDraft })),
      discard: vi.fn(),
      disconnectEdge: vi.fn(() => ({ ok: true, value: emptyDraft })),
      removeNode: vi.fn(() => ({ ok: true, value: emptyDraft })),
      removeSelection: vi.fn(() => ({ ok: true, value: emptyDraft })),
      save: vi.fn(async () => true),
      updateNodeField: vi.fn(() => ({
        ok: true,
        value: baseFactoryDefinition,
      })),
      ...actionOverrides,
    },
    blockedOperation: null,
    draftState: createDraftState(),
    graphState: null,
    pendingState: {
      hasChanges: false,
      pendingFactoryDefinition: baseFactoryDefinition,
    },
    projection: {
      edges: [],
      nodes: [],
    },
    documentSaveControls: {
      beginConfirmation: vi.fn(),
      cancelConfirmation: vi.fn(),
      clearSaveFeedback: vi.fn(),
    },
    saveState: {
      canSave: false,
      documentSave: { status: "idle" },
      isStale: false,
    },
    validationState: {
      errors: [],
      isValid: true,
    },
  };
}

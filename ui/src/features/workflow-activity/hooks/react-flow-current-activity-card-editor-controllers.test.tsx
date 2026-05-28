// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: existing current-activity editor-controller coverage stayed intact during feature-root migration.
import { act, renderHook } from "@testing-library/react";

import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import { buildFactoryGraphTopologyFromDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-graph";
import type {
  FactoryGraphDraft,
  FactoryGraphTopology,
} from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { createEmptyFactoryGraphDraft } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/public";
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
      type: "MODEL_WORKSTATION",
      worker: "writer",
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
      targetAnchorId: "workstation-output-target",
      targetNodeId: "work-state:story:done",
      targetStateName: "done",
    },
    {
      edgeKind: "workstation-on-continue",
      sourceAnchorId: "workstation-on-continue-source",
      targetAnchorId: "workstation-on-continue-target",
      targetNodeId: "work-state:story:queued",
      targetStateName: "queued",
    },
    {
      edgeKind: "workstation-on-failure",
      sourceAnchorId: "workstation-on-failure-source",
      targetAnchorId: "workstation-on-failure-target",
      targetNodeId: "work-state:story:queued",
      targetStateName: "queued",
    },
    {
      edgeKind: "workstation-on-rejection",
      sourceAnchorId: "workstation-on-rejection-source",
      targetAnchorId: "workstation-on-rejection-target",
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
      }),
    );

    act(() => {
      result.current.handleConnectionAnchorClick({
        anchorId: "workstation-output-target",
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
        targetHandle: "workstation-on-continue-target",
      });
    });

    expect(result.current.connectionNotice).toBe(
      "Failure connections from review cannot connect to Continue on story:done.",
    );
    expect(editableGraph.actions.connectNodes).toHaveBeenCalledTimes(1);
  });

  it.each([
    {
      confirmLabel: "Remove review success route",
      edgeId: "workstation-output:workstation:review->work-state:story:done",
      title: "Remove review success route?",
    },
    {
      confirmLabel: "Remove gpu resource link",
      edgeId: "workstation-resource:resource:gpu->workstation:review",
      title: "Remove gpu resource link?",
    },
    {
      confirmLabel: "Remove writer assignment",
      edgeId: "worker-assignment:worker:writer->workstation:review",
      title: "Remove writer assignment?",
    },
  ])("opens removable $edgeId confirmations and applies confirmed edge removals", ({
    confirmLabel,
    edgeId,
    title,
  }) => {
    const reset = vi.fn();
    const draftState = createDraftState();
    const editableGraph = createEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
        editableGraph,
        saveEditableDefinition: {
          reset,
          status: "idle",
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorEdgeDelete(edgeId);
    });

    expect(reset).toHaveBeenCalledTimes(1);
    expect(result.current.pendingRemovalIntent).toMatchObject({
      confirmLabel,
      title,
    });

    act(() => {
      result.current.handleConfirmRemoval();
    });

    expect(editableGraph.actions.disconnectEdge).toHaveBeenCalledWith(edgeId);
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it("surfaces blocked node-removal reasons through the shared removal state", () => {
    const reset = vi.fn();
    const draftState = createDraftState();
    const editableGraph = createEditableGraph({
      removeNode: vi.fn(() => ({
        message:
          "This worker is still assigned to 1 workstation. Reassign or remove those workstations before deleting writer.",
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
        saveEditableDefinition: {
          reset,
          status: "idle",
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorNodeDelete("worker:writer");
    });

    expect(reset).toHaveBeenCalledTimes(1);
    expect(editableGraph.actions.removeNode).toHaveBeenCalledWith(
      "worker:writer",
    );
    expect(result.current.blockedRemovalReason).toBe(
      "This worker is still assigned to 1 workstation. Reassign or remove those workstations before deleting writer.",
    );
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it.each([
    {
      confirmLabel: "Delete cache resource",
      nodeId: "resource:cache",
      title: "Remove cache resource?",
    },
    {
      confirmLabel: "Delete editor worker",
      nodeId: "worker:editor",
      title: "Remove editor worker?",
    },
  ])("opens removable $nodeId confirmations and applies confirmed node removals", ({
    confirmLabel,
    nodeId,
    title,
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
        saveEditableDefinition: {
          reset,
          status: "idle",
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorNodeDelete(nodeId);
    });

    expect(reset).toHaveBeenCalledTimes(1);
    expect(result.current.pendingRemovalIntent).toMatchObject({
      confirmLabel,
      title,
    });

    act(() => {
      result.current.handleConfirmRemoval();
    });

    expect(editableGraph.actions.removeNode).toHaveBeenCalledWith(nodeId);
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
    saveState: {
      canSave: false,
      isSaving: false,
      isStale: false,
      lastError: null,
      lastSuccess: false,
    },
    validationState: {
      errors: [],
      isValid: true,
    },
  };
}

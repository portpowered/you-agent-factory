// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: existing current-activity editor-controller coverage stayed intact during feature-root migration.
import { act, renderHook } from "@testing-library/react";

import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import { createEmptyFactoryGraphDraft } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { buildFactoryGraphTopologyFromDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-graph";
import type {
  FactoryGraphDraft,
  FactoryGraphTopology,
} from "../../factory-graph-editor/lib/factory-graph-draft-types";
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
  factoryDefinition: baseFactoryDefinition,
  version: {
    logical: 4,
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
  ])(
    "commits %s keyboard anchor connections and clears the pending source",
    ({
      edgeKind,
      sourceAnchorId,
      targetAnchorId,
      targetNodeId,
      targetStateName,
    }) => {
      const graph = createDraftState().graph;
      const updateDraft = vi.fn(
        (updater: (draft: FactoryGraphDraft) => FactoryGraphDraft) =>
          updater(createEmptyFactoryGraphDraft()),
      );
      const draftState = createDraftState({
        graph: {
          ...graph,
          edges: graph.edges.filter((edge) => edge.kind !== edgeKind),
        },
        updateDraft,
      });

      const { result } = renderHook(() =>
        useFactoryGraphConnectionController({
          activeTool: "connect",
          canInteractWithEditor: true,
          draftState,
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

      expect(updateDraft).toHaveBeenCalledTimes(1);
      expect(updateDraft.mock.results[0]?.value.edgeChanges.additions).toEqual([
        {
          kind: edgeKind,
          source: {
            kind: "workstation",
            name: "review",
          },
          target: {
            kind: "work-state",
            stateName: targetStateName,
            workTypeName: "story",
          },
        },
      ]);
      expect(result.current.connectionNotice).toBeNull();
      expect(result.current.pendingConnectionSource).toBeNull();
    },
  );

  it("shows actionable connection notices for invalid connection paths", () => {
    const updateDraft = vi.fn();
    const draftState = createDraftState({ updateDraft });

    const { result } = renderHook(() =>
      useFactoryGraphConnectionController({
        activeTool: "connect",
        canInteractWithEditor: true,
        draftState,
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
    expect(updateDraft).not.toHaveBeenCalled();
  });

  it("opens removable edge confirmations and applies confirmed edge removals", () => {
    const updateDraft = vi.fn(
      (updater: (draft: FactoryGraphDraft) => FactoryGraphDraft) =>
        updater(createEmptyFactoryGraphDraft()),
    );
    const reset = vi.fn();
    const draftState = createDraftState({ updateDraft });

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
        saveEditableDefinition: {
          reset,
          status: "idle",
        } as never,
      }),
    );

    act(() => {
      result.current.handleEditorEdgeDelete(
        "workstation-output:workstation:review->work-state:story:done",
      );
    });

    expect(reset).toHaveBeenCalledTimes(1);
    expect(result.current.pendingRemovalIntent).toMatchObject({
      confirmLabel: "Remove review success route",
      title: "Remove review success route?",
    });

    act(() => {
      result.current.handleConfirmRemoval();
    });

    expect(updateDraft).toHaveBeenCalledTimes(1);
    expect(updateDraft.mock.results[0]?.value.edgeChanges.removals).toEqual([
      {
        kind: "workstation-output",
        source: {
          kind: "workstation",
          name: "review",
        },
        target: {
          kind: "work-state",
          stateName: "done",
          workTypeName: "story",
        },
      },
    ]);
    expect(result.current.pendingRemovalIntent).toBeNull();
  });

  it("surfaces blocked node-removal reasons through the shared removal state", () => {
    const reset = vi.fn();
    const draftState = createDraftState();

    const { result } = renderHook(() =>
      useFactoryGraphRemovalController({
        activeTool: "delete",
        canInteractWithEditor: true,
        draftState,
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
    expect(result.current.blockedRemovalReason).toBe(
      "This worker is still assigned to 1 workstation. Reassign or remove those workstations before deleting writer.",
    );
    expect(result.current.pendingRemovalIntent).toBeNull();
  });
});

function createDraftState({
  draft = createEmptyFactoryGraphDraft(),
  graph = buildFactoryGraphTopologyFromDefinition(baseFactoryDefinition),
  updateDraft = vi.fn(),
}: {
  draft?: FactoryGraphDraft;
  graph?: FactoryGraphTopology;
  updateDraft?: ReturnType<typeof vi.fn>;
} = {}) {
  return {
    baseDocument: editableDocument,
    draft,
    graph,
    hasChanges: false,
    latestDocument: editableDocument,
    pendingFactoryDefinition: baseFactoryDefinition,
    replaceDraft: vi.fn(),
    resetDraft: vi.fn(),
    source: "editable-definition" as const,
    updateDraft,
    validationErrors: [],
  };
}

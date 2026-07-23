import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import { buildPendingFactoryDefinition } from "../draft/factory-graph-draft-apply";
import { createEmptyFactoryGraphDraft } from "../draft/factory-graph-draft-types";
import { removeFactoryGraphSelection } from "../operations/factory-graph-operations";
import { createEmptyFactoryGraphEditorSelection } from "./factory-graph-editor-selection";
import {
  applyFactoryGraphSelectionBatchRemoval,
  buildFactoryGraphSelectionBatchRemovalPlan,
  hasDeletableFactoryGraphSelection,
  pruneFactoryGraphEditorSelectionAfterRemoval,
} from "./factory-graph-editor-selection-batch-delete";

const baseFactoryDefinition: CanonicalFactoryDefinition = {
  name: "Current Factory",
  resources: [{ capacity: 2, name: "gpu" }],
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
    {
      model: "gpt-5",
      name: "editor",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        { name: "queued", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workstations: [
    {
      inputs: [{ state: "queued", workType: "story" }],
      name: "review",
      outputs: [{ state: "done", workType: "story" }],
      resources: [{ capacity: 2, name: "gpu" }],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

describe("factory graph editor selection batch delete planning", () => {
  it("builds a single-node removal plan without confirmation for unassigned workers", () => {
    const draft = createEmptyFactoryGraphDraft();
    draft.additions.workers.push({
      model: "gpt-5",
      name: "editor",
      type: "MODEL_WORKER",
    });

    const plan = buildFactoryGraphSelectionBatchRemovalPlan({
      baseFactoryDefinition,
      draft,
      selection: {
        edgeIds: [],
        nodeIds: ["worker:editor"],
      },
    });

    expect(plan).toMatchObject({
      nodeIds: ["worker:editor"],
      edgeIds: [],
      confirmation: undefined,
    });
  });

  it("requires confirmation when deleting a workstation with connected edges", () => {
    const draft = createEmptyFactoryGraphDraft();

    const plan = buildFactoryGraphSelectionBatchRemovalPlan({
      baseFactoryDefinition,
      draft,
      selection: {
        edgeIds: [],
        nodeIds: ["workstation:review"],
      },
    });

    expect(plan).toMatchObject({
      nodeIds: ["workstation:review"],
      confirmation: {
        confirmLabel: "Delete review workstation",
        title: "Remove review workstation?",
      },
    });
  });

  it("blocks batch delete when any selected node is ineligible", () => {
    const draft = createEmptyFactoryGraphDraft();

    const plan = buildFactoryGraphSelectionBatchRemovalPlan({
      baseFactoryDefinition,
      draft,
      selection: {
        edgeIds: [],
        nodeIds: ["worker:writer", "resource:gpu"],
      },
    });

    expect(plan?.ineligibleReason).toContain("still assigned");
  });
});

describe("factory graph editor selection batch delete apply", () => {
  it("removes workstation nodes and cascades incident edge removals", () => {
    const draft = createEmptyFactoryGraphDraft();
    const nextDraft = applyFactoryGraphSelectionBatchRemoval(
      draft,
      baseFactoryDefinition,
      {
        edgeIds: [
          "workstation-output:workstation:review->work-state:story:done",
        ],
        nodeIds: ["workstation:review"],
      },
    );

    expect(nextDraft.removals.workstations).toEqual(["review"]);
    expect(nextDraft.edgeChanges.removals).toEqual([]);
  });

  it("removes multiple draft-only resources in one draft mutation", () => {
    const draft = createEmptyFactoryGraphDraft();
    draft.additions.resources.push(
      { capacity: 1, name: "cache" },
      { capacity: 1, name: "queue" },
    );

    const result = removeFactoryGraphSelection({
      baseFactoryDefinition,
      draft,
      edgeIds: [],
      nodeIds: ["resource:cache", "resource:queue"],
    });

    expect(result.ok).toBe(true);
    if (!result.ok) {
      return;
    }

    expect(result.value.additions.resources).toEqual([]);
  });

  it("reports deletable mixed node and edge selections", () => {
    const draft = createEmptyFactoryGraphDraft();

    expect(
      hasDeletableFactoryGraphSelection({
        baseFactoryDefinition,
        draft,
        selection: {
          selectedEdgeIds: new Set([
            "workstation-output:workstation:review->work-state:story:done",
          ]),
          selectedNodeIds: new Set(["resource:gpu"]),
        },
      }),
    ).toBe(true);
  });

  it("prunes deleted graph ids from editor-local selection", () => {
    const state = {
      ...createEmptyFactoryGraphEditorSelection(),
      selectedEdgeIds: new Set([
        "workstation-output:workstation:review->work-state:story:done",
      ]),
      selectedNodeIds: new Set(["resource:gpu", "worker:editor"]),
      primaryTarget: { kind: "node" as const, id: "resource:gpu" },
    };

    const nextState = pruneFactoryGraphEditorSelectionAfterRemoval(state, {
      edgeIds: ["workstation-output:workstation:review->work-state:story:done"],
      nodeIds: ["resource:gpu"],
    });

    expect([...nextState.selectedNodeIds]).toEqual(["worker:editor"]);
    expect([...nextState.selectedEdgeIds]).toEqual([]);
    expect(nextState.primaryTarget).toEqual({
      kind: "node",
      id: "worker:editor",
    });
  });

  it("leaves pending factory topology without deleted entities after batch apply", () => {
    const draft = createEmptyFactoryGraphDraft();
    draft.additions.resources.push({ capacity: 1, name: "cache" });

    const result = removeFactoryGraphSelection({
      baseFactoryDefinition,
      draft,
      edgeIds: [],
      nodeIds: ["resource:cache"],
    });

    expect(result.ok).toBe(true);
    if (!result.ok) {
      return;
    }

    const pending = buildPendingFactoryDefinition(
      baseFactoryDefinition,
      result.value,
    );
    expect(pending?.resources?.map((resource) => resource.name)).not.toContain(
      "cache",
    );
  });
});

import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import {
  applyFactoryGraphEntityRemoval,
  buildFactoryGraphEdgeRemovalIntent,
  buildFactoryGraphRemovalIntent,
  buildPendingFactoryDefinition,
  collectPendingRemovalEdgeIds,
  collectPendingRemovalNodeIds,
  createEmptyFactoryGraphDraft,
} from "./factory-graph-draft";

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

describe("factory graph editor removals", () => {
  it("summarizes workstation deletion impact and keeps it visible as pending removal", () => {
    const draft = createEmptyFactoryGraphDraft();
    const removalIntent = buildFactoryGraphRemovalIntent({
      baseFactoryDefinition,
      draft,
      nodeId: "workstation:review",
    });

    expect(removalIntent).toMatchObject({
      confirmDescription: "This will remove 4 graph edges.",
      confirmLabel: "Delete review workstation",
      title: "Remove review workstation?",
    });

    const nextDraft = applyFactoryGraphEntityRemoval(
      draft,
      baseFactoryDefinition,
      {
        kind: "workstation",
        name: "review",
      },
    );

    expect(nextDraft.removals.workstations).toEqual(["review"]);
    expect(
      Array.from(collectPendingRemovalNodeIds(baseFactoryDefinition, nextDraft)),
    ).toEqual(["workstation:review"]);
    expect(
      Array.from(collectPendingRemovalEdgeIds(baseFactoryDefinition, nextDraft)),
    ).toEqual([
      "worker-assignment:worker:writer->workstation:review",
      "workstation-input:work-state:story:queued->workstation:review",
      "workstation-output:workstation:review->work-state:story:done",
      "workstation-resource:resource:gpu->workstation:review",
    ]);
  });

  it("blocks deleting workers that are still assigned to workstations", () => {
    const draft = createEmptyFactoryGraphDraft();

    expect(
      buildFactoryGraphRemovalIntent({
        baseFactoryDefinition,
        draft,
        nodeId: "worker:writer",
      }),
    ).toMatchObject({
      ineligibleReason:
        "This worker is still assigned to 1 workstation. Reassign or remove those workstations before deleting writer.",
    });
  });

  it("removes work-state references from surviving workstations when deleting a state", () => {
    const draft = createEmptyFactoryGraphDraft();
    const nextDraft = applyFactoryGraphEntityRemoval(
      draft,
      baseFactoryDefinition,
      {
        kind: "work-state",
        stateName: "done",
        workTypeName: "story",
      },
    );

    expect(nextDraft.removals.workStates).toEqual([
      {
        stateName: "done",
        workTypeName: "story",
      },
    ]);
    expect(nextDraft.edgeChanges.removals).toEqual([
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
    expect(
      buildPendingFactoryDefinition(baseFactoryDefinition, nextDraft)?.workstations,
    ).toEqual([
      {
        inputs: [
          {
            state: "queued",
            workType: "story",
          },
        ],
        name: "review",
        outputs: [],
        resources: [
          {
            capacity: 2,
            name: "gpu",
          },
        ],
        type: "MODEL_WORKSTATION",
        worker: "writer",
      },
    ]);
  });

  it("describes routing impact before removing workstation transition edges", () => {
    const draft = createEmptyFactoryGraphDraft();

    expect(
      buildFactoryGraphEdgeRemovalIntent({
        baseFactoryDefinition,
        draft,
        edgeId:
          "workstation-output:workstation:review->work-state:story:done",
      }),
    ).toMatchObject({
      confirmLabel: "Remove review success route",
      title: "Remove review success route?",
      confirmDescription:
        "This will remove the success route from review to story:done.",
    });
  });

  it("blocks direct removal of work-type membership edges", () => {
    const draft = createEmptyFactoryGraphDraft();

    expect(
      buildFactoryGraphEdgeRemovalIntent({
        baseFactoryDefinition,
        draft,
        edgeId: "work-type-state:work-type:story->work-state:story:queued",
      }),
    ).toMatchObject({
      ineligibleReason:
        "Work type ordering edges are managed by work-state membership and cannot be removed directly.",
    });
  });
});

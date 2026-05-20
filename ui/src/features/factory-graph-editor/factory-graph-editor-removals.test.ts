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
      Array.from(
        collectPendingRemovalNodeIds(baseFactoryDefinition, nextDraft),
      ),
    ).toEqual(["workstation:review"]);
    expect(
      Array.from(
        collectPendingRemovalEdgeIds(baseFactoryDefinition, nextDraft),
      ),
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
      buildPendingFactoryDefinition(baseFactoryDefinition, nextDraft)
        ?.workstations,
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
        edgeId: "workstation-output:workstation:review->work-state:story:done",
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

  it("summarizes work-type and resource removals with dependent topology impact", () => {
    const draft = createEmptyFactoryGraphDraft();

    expect(
      buildFactoryGraphRemovalIntent({
        baseFactoryDefinition,
        draft,
        nodeId: "work-type:story",
      }),
    ).toMatchObject({
      confirmDescription:
        "This will remove 2 graph edges. story also owns 2 work states, which will be removed with it.",
      confirmLabel: "Delete story work-type",
      title: "Remove story work-type?",
    });

    expect(
      buildFactoryGraphRemovalIntent({
        baseFactoryDefinition,
        draft,
        nodeId: "resource:gpu",
      }),
    ).toMatchObject({
      confirmDescription:
        "This will remove 1 graph edge. Worker and workstation resource references that depend on gpu will be cleared from the pending draft.",
      confirmLabel: "Delete gpu resource",
      title: "Remove gpu resource?",
    });
  });

  it("returns no removal intent for unknown nodes and unknown edges", () => {
    const draft = createEmptyFactoryGraphDraft();

    expect(
      buildFactoryGraphRemovalIntent({
        baseFactoryDefinition,
        draft,
        nodeId: "workstation:missing",
      }),
    ).toBeNull();
    expect(
      buildFactoryGraphEdgeRemovalIntent({
        baseFactoryDefinition,
        draft,
        edgeId: "workstation-output:missing",
      }),
    ).toBeNull();
  });

  it("removes draft-only entities instead of creating server-backed removals", () => {
    const draft = createEmptyFactoryGraphDraft();
    draft.additions.resources.push({ capacity: 1, name: "cache" });
    draft.additions.workers.push({
      model: "gpt-5",
      name: "editor",
      type: "MODEL_WORKER",
    });
    draft.additions.workTypes.push({ name: "bug", states: [] });
    draft.additions.workStates.push({
      state: { name: "triage", type: "PROCESSING" },
      workTypeName: "bug",
    });
    draft.additions.workstations.push({
      inputs: [],
      name: "triage",
      outputs: [],
      type: "MODEL_WORKSTATION",
      worker: "editor",
    });
    draft.edgeChanges.additions.push({
      kind: "worker-resource",
      source: { kind: "resource", name: "cache" },
      target: { kind: "worker", name: "editor" },
    });

    const withoutResource = applyFactoryGraphEntityRemoval(
      draft,
      baseFactoryDefinition,
      { kind: "resource", name: "cache" },
    );
    expect(withoutResource.additions.resources).toEqual([]);
    expect(withoutResource.removals.resources).toEqual([]);
    expect(withoutResource.edgeChanges.additions).toEqual([]);

    const withoutWorkType = applyFactoryGraphEntityRemoval(
      draft,
      baseFactoryDefinition,
      { kind: "work-type", name: "bug" },
    );
    expect(withoutWorkType.additions.workTypes).toEqual([]);
    expect(withoutWorkType.additions.workStates).toEqual([]);
    expect(withoutWorkType.removals.workTypes).toEqual([]);

    const withoutWorker = applyFactoryGraphEntityRemoval(
      draft,
      baseFactoryDefinition,
      { kind: "worker", name: "editor" },
    );
    expect(withoutWorker.additions.workers).toEqual([]);
    expect(withoutWorker.removals.workers).toEqual([]);

    const withoutWorkstation = applyFactoryGraphEntityRemoval(
      draft,
      baseFactoryDefinition,
      { kind: "workstation", name: "triage" },
    );
    expect(withoutWorkstation.additions.workstations).toEqual([]);
    expect(withoutWorkstation.removals.workstations).toEqual([]);
  });
});

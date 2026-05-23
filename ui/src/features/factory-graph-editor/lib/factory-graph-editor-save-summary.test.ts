import { createEmptyFactoryGraphDraft } from "./factory-graph-draft-types";
import { buildFactoryGraphSaveSummary } from "./factory-graph-editor-save-summary";

describe("buildFactoryGraphSaveSummary", () => {
  it("summarizes created, deleted, and changed graph items", () => {
    const draft = createEmptyFactoryGraphDraft();
    draft.additions.workers.push({
      model: "gpt-5-mini",
      name: "writer",
      type: "MODEL_WORKER",
    });
    draft.additions.workstations.push({
      body: "Draft work.",
      inputs: [],
      name: "review",
      outputs: [],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    });
    draft.removals.resources.push("gpu");
    draft.edgeChanges.additions.push({
      kind: "worker-assignment",
      source: {
        kind: "worker",
        name: "writer",
      },
      target: {
        kind: "workstation",
        name: "review",
      },
    });

    expect(buildFactoryGraphSaveSummary(draft)).toEqual({
      changedEdges: 1,
      createdEntities: 2,
      description:
        "This save will apply 2 created entities, 1 deleted entity and 1 changed edge.",
      removedEntities: 1,
    });
  });

  it("returns an empty summary when the draft has no pending changes", () => {
    expect(buildFactoryGraphSaveSummary(createEmptyFactoryGraphDraft())).toEqual(
      {
        changedEdges: 0,
        createdEntities: 0,
        description: "No graph changes are pending.",
        removedEntities: 0,
      },
    );
  });
});

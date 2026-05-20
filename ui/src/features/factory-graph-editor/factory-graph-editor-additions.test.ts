import {
  applyFactoryGraphAddEntityDraft,
  createFactoryGraphAddEntityDraft,
  validateFactoryGraphAddEntityDraft,
} from "./factory-graph-editor-additions";
import { createEmptyFactoryGraphDraft, type CanonicalFactoryDefinition } from "./factory-graph-draft";

const baseFactoryDefinition: CanonicalFactoryDefinition = {
  name: "Current Factory",
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
      ],
    },
  ],
  workstations: [
    {
      inputs: [],
      name: "draft",
      outputs: [],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

describe("factory graph editor additions", () => {
  it("seeds workstation and work-state drafts from the current pending definition", () => {
    expect(
      createFactoryGraphAddEntityDraft("workstation", baseFactoryDefinition),
    ).toMatchObject({
      kind: "workstation",
      workerName: "writer",
    });
    expect(
      createFactoryGraphAddEntityDraft("work-state", baseFactoryDefinition),
    ).toMatchObject({
      kind: "work-state",
      stateType: "PROCESSING",
      workTypeName: "story",
    });
  });

  it("rejects duplicate identifiers and structurally invalid add forms", () => {
    expect(
      validateFactoryGraphAddEntityDraft(
        {
          kind: "worker",
          model: "",
          name: "writer",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({
      model: "Enter a model identifier for the new worker.",
      name: 'A worker named "writer" already exists in the draft.',
    });

    expect(
      validateFactoryGraphAddEntityDraft(
        {
          capacity: "0",
          kind: "resource",
          name: "gpu",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({
      capacity: "Resource capacity must be a whole number greater than zero.",
    });
  });

  it("appends pending entities to the graph draft in canonical factory shape", () => {
    const nextDraft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        body: "Review the story output.",
        kind: "workstation",
        name: "review",
        workerName: "writer",
      },
    );

    expect(nextDraft.additions.workstations).toEqual([
      {
        body: "Review the story output.",
        inputs: [],
        name: "review",
        outputs: [],
        type: "MODEL_WORKSTATION",
        worker: "writer",
      },
    ]);
  });
});

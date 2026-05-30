import {
  applyFactoryGraphAddEntityDraft,
  createFactoryGraphAddEntityDraft,
  validateFactoryGraphAddEntityDraft,
} from "./factory-graph-editor-additions";
import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { createEmptyFactoryGraphDraft } from "./factory-graph-draft-types";

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
      behavior: "STANDARD",
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
          behavior: "POLLER",
          body: "",
          kind: "workstation",
          name: "linear-poller",
          workerName: "writer",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({
      behavior: "Poller workstations must use a script or hosted worker.",
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

  it("appends poller behavior to new workstations in canonical factory shape", () => {
    const nextDraft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        behavior: "POLLER",
        body: "Review the story output.",
        kind: "workstation",
        name: "review",
        workerName: "writer",
      },
    );

    expect(nextDraft.additions.workstations).toEqual([
      {
        behavior: "POLLER",
        body: "Review the story output.",
        inputs: [],
        name: "review",
        outputs: [],
        type: "MODEL_WORKSTATION",
        worker: "writer",
      },
    ]);
  });

  it("omits explicit standard behavior for new workstations", () => {
    const nextDraft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        behavior: "STANDARD",
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

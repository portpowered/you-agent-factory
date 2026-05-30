import { baseFactoryDefinition } from "./factory-graph-draft.test-helpers";
import { createEmptyFactoryGraphDraft } from "./factory-graph-draft-types";
import {
  validateFactoryGraphDraft,
  validateFactoryGraphDraftSave,
} from "./factory-graph-draft-validation";

const logicalMoveFactoryDefinition = {
  ...baseFactoryDefinition,
  workstations: [
    ...(baseFactoryDefinition.workstations ?? []),
    {
      body: "Move work downstream.",
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "router",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      type: "LOGICAL_MOVE" as const,
      worker: "",
    },
  ],
};

const modelWorkstationWithoutWorkerFactoryDefinition = {
  ...baseFactoryDefinition,
  workstations: [
    {
      body: "Draft the story.",
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "draft",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      type: "MODEL_WORKSTATION" as const,
      worker: "",
    },
  ],
};

it("accepts save validation for LOGICAL_MOVE workstations without worker assignments", () => {
  const validationErrors = validateFactoryGraphDraftSave(
    logicalMoveFactoryDefinition,
    createEmptyFactoryGraphDraft(),
  );

  expect(
    validationErrors.filter(
      (error) =>
        error.code === "MISSING_REQUIRED_FIELD" && error.field === "worker",
    ),
  ).toEqual([]);
});

it("rejects save validation for worker-backed workstations without worker assignments", () => {
  const validationErrors = validateFactoryGraphDraftSave(
    modelWorkstationWithoutWorkerFactoryDefinition,
    createEmptyFactoryGraphDraft(),
  );

  expect(validationErrors).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        code: "MISSING_REQUIRED_FIELD",
        field: "worker",
        target: {
          kind: "node",
          id: "workstation:draft",
        },
      }),
    ]),
  );
});

it("skips missing-worker validation when adding a LOGICAL_MOVE workstation draft", () => {
  const draft = createEmptyFactoryGraphDraft();
  draft.additions.workstations.push({
    body: "Route approved work downstream.",
    inputs: [
      {
        state: "done",
        workType: "story",
      },
    ],
    name: "downstream",
    outputs: [
      {
        state: "done",
        workType: "story",
      },
    ],
    type: "LOGICAL_MOVE",
    worker: "",
  });

  const validationErrors = validateFactoryGraphDraftSave(
    baseFactoryDefinition,
    draft,
  );

  expect(
    validationErrors.filter(
      (error) =>
        error.code === "MISSING_REQUIRED_FIELD" && error.field === "worker",
    ),
  ).toEqual([]);
});

it("requires worker assignment when adding a MODEL_WORKSTATION draft", () => {
  const draft = createEmptyFactoryGraphDraft();
  draft.additions.workstations.push({
    body: "Review the drafted story.",
    inputs: [],
    name: "review",
    outputs: [],
    type: "MODEL_WORKSTATION",
    worker: "",
  });

  const validationErrors = validateFactoryGraphDraft(
    baseFactoryDefinition,
    draft,
  );

  expect(validationErrors).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        code: "MISSING_REQUIRED_FIELD",
        field: "worker",
        target: {
          kind: "node",
          id: "workstation:review",
        },
      }),
    ]),
  );
});

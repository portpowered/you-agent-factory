import { describe, expect, it } from "vitest";
import type { CanonicalFactoryDefinition } from "./factory-graph-draft-types";
import { materializeFactoryGraphEntityIdsForSave } from "./factory-graph-public-ids";

describe("factory graph public ids", () => {
  it("materializes missing ids for every graphable entity namespace", () => {
    const factory = materializeFactoryGraphEntityIdsForSave(legacyFactory);

    expect(factory.resources).toEqual([
      expect.objectContaining({ id: "model-slot", name: "model-slot" }),
      expect.objectContaining({ id: "review-slot", name: "review-slot" }),
    ]);
    expect(factory.workers).toEqual([
      expect.objectContaining({ id: "writer", name: "writer" }),
      expect.objectContaining({ id: "reviewer", name: "reviewer" }),
    ]);
    expect(factory.workTypes?.[0]).toEqual(
      expect.objectContaining({ id: "story", name: "story" }),
    );
    expect(factory.workTypes?.[0]?.states).toEqual([
      expect.objectContaining({ id: "queued", name: "queued" }),
      expect.objectContaining({ id: "done", name: "done" }),
    ]);
    expect(factory.workstations).toEqual([
      expect.objectContaining({ id: "draft", name: "draft" }),
    ]);
    expect(legacyFactory.resources?.[0]?.id).toBeUndefined();
  });

  it("keeps explicit ids stable across subsequent saves and renames", () => {
    const firstSave = materializeFactoryGraphEntityIdsForSave(idBackedFactory);
    const secondSave = materializeFactoryGraphEntityIdsForSave(firstSave);

    expect(firstSave.resources?.[0]).toMatchObject({
      id: "resource-id-model-slot",
      name: "renamed-model-slot",
    });
    expect(firstSave.workers?.[0]).toMatchObject({
      id: "worker-id-writer",
      name: "renamed-writer",
    });
    expect(firstSave.workTypes?.[0]).toMatchObject({
      id: "work-type-id-story",
      name: "renamed-story",
    });
    expect(firstSave.workTypes?.[0]?.states[0]).toMatchObject({
      id: "state-id-queued",
      name: "renamed-queued",
    });
    expect(firstSave.workstations?.[0]).toMatchObject({
      id: "workstation-id-draft",
      name: "renamed-draft",
    });
    expect(secondSave).toEqual(firstSave);
  });
});

const legacyFactory: CanonicalFactoryDefinition = {
  name: "legacy",
  resources: [
    { capacity: 4, name: "model-slot" },
    { capacity: 2, name: "review-slot" },
  ],
  workers: [
    { model: "gpt-5", name: "writer", type: "MODEL_WORKER" },
    { command: "node", name: "reviewer", type: "SCRIPT_WORKER" },
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
      name: "draft",
      outputs: [{ state: "done", workType: "story" }],
      worker: "writer",
    },
  ],
};

const idBackedFactory: CanonicalFactoryDefinition = {
  ...legacyFactory,
  resources: [
    { capacity: 4, id: "resource-id-model-slot", name: "renamed-model-slot" },
  ],
  workers: [
    { id: "worker-id-writer", model: "gpt-5", name: "renamed-writer" },
  ],
  workTypes: [
    {
      id: "work-type-id-story",
      name: "renamed-story",
      states: [
        { id: "state-id-queued", name: "renamed-queued", type: "INITIAL" },
        { id: "state-id-done", name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workstations: [
    {
      id: "workstation-id-draft",
      inputs: [{ state: "renamed-queued", workType: "renamed-story" }],
      name: "renamed-draft",
      outputs: [{ state: "done", workType: "renamed-story" }],
      worker: "renamed-writer",
    },
  ],
};

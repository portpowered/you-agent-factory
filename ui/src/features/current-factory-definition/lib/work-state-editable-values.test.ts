import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  applyEditableWorkStateDraft,
  editableWorkStateDraftFromValues,
  resolveEditableWorkStateValues,
} from "./work-state-editable-values";

const factoryWithWorkStateRoutes: CanonicalFactoryDefinition = {
  name: "Current Factory",
  workers: [
    { modelProvider: "CODEX", name: "reviewer", type: "MODEL_WORKER" },
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
      id: "plan",
      inputs: [{ state: "queued", workType: "story" }],
      name: "Plan",
      onContinue: [{ state: "queued", workType: "story" }],
      onFailure: [{ state: "queued", workType: "story" }],
      onRejection: [{ state: "queued", workType: "story" }],
      outputs: [{ state: "done", workType: "story" }],
      worker: "reviewer",
    },
    {
      classificationRoutes: [
        {
          label: "ship",
          outputs: [{ state: "queued", workType: "story" }],
        },
      ],
      id: "classify",
      inputs: [{ state: "queued", workType: "story" }],
      name: "Classify",
      worker: "reviewer",
    },
    {
      id: "other",
      inputs: [{ state: "queued", workType: "task" }],
      name: "Other",
      worker: "reviewer",
    },
  ],
};

describe("resolveEditableWorkStateValues", () => {
  it("returns work state values for a factory-backed place id", () => {
    expect(
      resolveEditableWorkStateValues(
        factoryWithWorkStateRoutes,
        "story:queued",
      ),
    ).toEqual({
      stateName: "queued",
      stateNamesInWorkType: ["queued", "done"],
      stateType: "INITIAL",
      workTypeName: "story",
    });
  });

  it("returns null when the place id does not resolve to a factory work state", () => {
    expect(
      resolveEditableWorkStateValues(
        factoryWithWorkStateRoutes,
        "story:missing",
      ),
    ).toBeNull();
  });
});

describe("applyEditableWorkStateDraft", () => {
  it("renames the work state and propagates workstation route references", () => {
    const updatedFactory = applyEditableWorkStateDraft(
      factoryWithWorkStateRoutes,
      "story:queued",
      {
        name: "ready",
        type: "INITIAL",
      },
    );

    expect(updatedFactory?.workTypes?.[0]?.states).toEqual([
      { name: "ready", type: "INITIAL" },
      { name: "done", type: "TERMINAL" },
    ]);
    expect(updatedFactory?.workstations?.[0]).toMatchObject({
      inputs: [{ state: "ready", workType: "story" }],
      onContinue: [{ state: "ready", workType: "story" }],
      onFailure: [{ state: "ready", workType: "story" }],
      onRejection: [{ state: "ready", workType: "story" }],
      outputs: [{ state: "done", workType: "story" }],
    });
    expect(
      updatedFactory?.workstations?.[1]?.classificationRoutes?.[0]?.outputs,
    ).toEqual([{ state: "ready", workType: "story" }]);
    expect(updatedFactory?.workstations?.[2]?.inputs).toEqual([
      { state: "queued", workType: "task" },
    ]);
  });

  it("preserves lifecycle type when only the name changes", () => {
    const updatedFactory = applyEditableWorkStateDraft(
      factoryWithWorkStateRoutes,
      "story:done",
      editableWorkStateDraftFromValues({
        stateName: "done",
        stateNamesInWorkType: ["queued", "done"],
        stateType: "TERMINAL",
        workTypeName: "story",
      }),
    );

    expect(updatedFactory?.workTypes?.[0]?.states[1]).toEqual({
      name: "done",
      type: "TERMINAL",
    });
  });

  it("returns null when the place id does not resolve", () => {
    expect(
      applyEditableWorkStateDraft(factoryWithWorkStateRoutes, "story:missing", {
        name: "ready",
        type: "INITIAL",
      }),
    ).toBeNull();
  });
});

import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  applyEditableWorkTypeDraft,
  editableWorkTypeDraftFromValues,
  resolveEditableWorkTypeValues,
} from "./work-type-editable-values";

describe("resolveEditableWorkTypeValues", () => {
  it("returns the selected work type values when the type exists", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          handlingBehavior: ["DEFAULT"],
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
    };

    expect(resolveEditableWorkTypeValues(factory, "story")).toEqual({
      handlingBehavior: ["DEFAULT"],
      states: [
        { name: "queued", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
      workTypeName: "story",
    });
  });

  it("returns null when the work type is missing from the factory document", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workTypes: [],
    };

    expect(resolveEditableWorkTypeValues(factory, "missing-type")).toBeNull();
  });
});

describe("applyEditableWorkTypeDraft", () => {
  it("propagates work type renames across workstation route references", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          name: "story",
          states: [{ name: "queued", type: "INITIAL" }],
        },
        {
          name: "bug",
          states: [{ name: "open", type: "INITIAL" }],
        },
      ],
      workstations: [
        {
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          onFailure: [{ state: "queued", workType: "story" }],
          onRejection: [{ state: "queued", workType: "story" }],
          outputs: [{ state: "queued", workType: "story" }],
          worker: "reviewer",
        },
        {
          id: "triage",
          classificationRoutes: [
            {
              label: "story",
              outputs: [{ state: "queued", workType: "story" }],
            },
          ],
          inputs: [{ state: "open", workType: "bug" }],
          name: "Triage",
          worker: "reviewer",
        },
      ],
      workers: [{ name: "reviewer", type: "MODEL_WORKER" }],
    };

    const updatedFactory = applyEditableWorkTypeDraft(factory, "story", {
      handlingBehavior: null,
      name: "feature",
    });

    expect(updatedFactory?.workTypes).toEqual([
      {
        name: "feature",
        states: [{ name: "queued", type: "INITIAL" }],
      },
      {
        name: "bug",
        states: [{ name: "open", type: "INITIAL" }],
      },
    ]);
    expect(updatedFactory?.workstations).toEqual([
      {
        id: "review",
        inputs: [{ state: "queued", workType: "feature" }],
        name: "Review",
        onFailure: [{ state: "queued", workType: "feature" }],
        onRejection: [{ state: "queued", workType: "feature" }],
        outputs: [{ state: "queued", workType: "feature" }],
        worker: "reviewer",
      },
      {
        id: "triage",
        classificationRoutes: [
          {
            label: "story",
            outputs: [{ state: "queued", workType: "feature" }],
          },
        ],
        inputs: [{ state: "open", workType: "bug" }],
        name: "Triage",
        worker: "reviewer",
      },
    ]);
  });

  it("transfers DEFAULT handling to the edited work type and clears other defaults", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          handlingBehavior: ["DEFAULT"],
          name: "story",
          states: [{ name: "queued", type: "INITIAL" }],
        },
        {
          name: "bug",
          states: [{ name: "open", type: "INITIAL" }],
        },
      ],
    };

    const updatedFactory = applyEditableWorkTypeDraft(factory, "bug", {
      handlingBehavior: ["DEFAULT"],
      name: "bug",
    });

    expect(updatedFactory?.workTypes).toEqual([
      {
        name: "story",
        states: [{ name: "queued", type: "INITIAL" }],
      },
      {
        handlingBehavior: ["DEFAULT"],
        name: "bug",
        states: [{ name: "open", type: "INITIAL" }],
      },
    ]);
  });

  it("clears DEFAULT from the edited work type when default is unchecked", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          handlingBehavior: ["DEFAULT"],
          name: "story",
          states: [{ name: "queued", type: "INITIAL" }],
        },
      ],
    };

    const updatedFactory = applyEditableWorkTypeDraft(factory, "story", {
      handlingBehavior: null,
      name: "story",
    });

    expect(updatedFactory?.workTypes).toEqual([
      {
        name: "story",
        states: [{ name: "queued", type: "INITIAL" }],
      },
    ]);
  });

  it("keeps pending work types with at most one DEFAULT after default transfer", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          handlingBehavior: ["DEFAULT"],
          name: "alpha",
          states: [{ name: "queued", type: "INITIAL" }],
        },
        {
          handlingBehavior: ["DEFAULT"],
          name: "beta",
          states: [{ name: "open", type: "INITIAL" }],
        },
      ],
    };

    const updatedFactory = applyEditableWorkTypeDraft(factory, "beta", {
      handlingBehavior: ["DEFAULT"],
      name: "beta",
    });

    const defaultCount = (updatedFactory?.workTypes ?? []).filter((workType) =>
      workType.handlingBehavior?.includes("DEFAULT"),
    ).length;

    expect(defaultCount).toBe(1);
    expect(updatedFactory?.workTypes?.[0]).toEqual({
      name: "alpha",
      states: [{ name: "queued", type: "INITIAL" }],
    });
    expect(updatedFactory?.workTypes?.[1]).toEqual({
      handlingBehavior: ["DEFAULT"],
      name: "beta",
      states: [{ name: "open", type: "INITIAL" }],
    });
  });

  it("round-trips handlingBehavior through draft helpers", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          handlingBehavior: ["DEFAULT"],
          name: "story",
          states: [{ name: "queued", type: "INITIAL" }],
        },
      ],
    };

    const values = resolveEditableWorkTypeValues(factory, "story");
    expect(values).not.toBeNull();
    if (!values) {
      return;
    }

    const draft = editableWorkTypeDraftFromValues(values);
    const updatedFactory = applyEditableWorkTypeDraft(factory, "story", draft);

    expect(updatedFactory?.workTypes?.[0]).toEqual({
      handlingBehavior: ["DEFAULT"],
      name: "story",
      states: [{ name: "queued", type: "INITIAL" }],
    });
  });
});

import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  applyEditableWorkTypeDraft,
  editableWorkTypeDraftFromValues,
  resolveEditableWorkTypeValues,
} from "./work-type-editable-values";

describe("applyEditableWorkTypeDraft default handling", () => {
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

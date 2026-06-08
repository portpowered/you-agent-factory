import { describe, expect, it } from "vitest";

import { buildFactoryGraphAddEntityMenuActions } from "./factory-graph-editor-add-menu";

describe("buildFactoryGraphAddEntityMenuActions", () => {
  it("lists doc as the first add action and disables work-state without work types", () => {
    const actions = buildFactoryGraphAddEntityMenuActions(
      { name: "Current Factory", workTypes: [] },
      "en",
    );

    expect(actions.map((action) => action.id)).toEqual([
      "doc",
      "workstation",
      "worker",
      "work-type",
      "work-state",
      "resource",
    ]);
    expect(actions.find((action) => action.id === "doc")).toMatchObject({
      label: "Doc",
    });
    expect(actions.find((action) => action.id === "work-state")?.disabled).toBe(
      true,
    );
  });

  it("enables work-state when the factory already has work types", () => {
    const actions = buildFactoryGraphAddEntityMenuActions(
      {
        name: "Current Factory",
        workTypes: [{ id: "wt-1", name: "Review" }],
      },
      "en",
    );

    expect(actions.find((action) => action.id === "work-state")?.disabled).toBe(
      false,
    );
  });
});

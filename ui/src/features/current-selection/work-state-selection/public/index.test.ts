import { describe, expect, it } from "vitest";

import * as workStateSelectionPublic from "./index";

describe("work-state-selection/public", () => {
  it("exports state node detail card, editable state types, and message helpers", () => {
    expect(
      workStateSelectionPublic.EditableWorkStateConfigurationHeaderActions,
    ).toBeTypeOf("function");
    expect(
      workStateSelectionPublic.EditableWorkStateSaveHeaderAction,
    ).toBeTypeOf("function");
    expect(workStateSelectionPublic.StateNodeDetailCard).toBeTypeOf("function");
    expect(workStateSelectionPublic.getWorkStateDetailMessages).toBeTypeOf(
      "function",
    );
  });

  it("does not export work state editing hooks", () => {
    expect(workStateSelectionPublic).not.toHaveProperty(
      "useEditableWorkStateConfigurationState",
    );
    expect(workStateSelectionPublic).not.toHaveProperty(
      "useSaveEditableWorkStateConfiguration",
    );
  });
});

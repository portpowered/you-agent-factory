import { describe, expect, it } from "vitest";

import * as currentSelectionBasePublic from "./index";

describe("current-selection base public barrel", () => {
  it("exports shared selection contracts and layout primitives", () => {
    expect(currentSelectionBasePublic.resolveDashboardSelection).toBeTypeOf(
      "function",
    );
    expect(currentSelectionBasePublic.selectDefaultSelection).toBeTypeOf(
      "function",
    );
    expect(currentSelectionBasePublic.resetSelectionHistoryStore).toBeTypeOf(
      "function",
    );
    expect(currentSelectionBasePublic.SelectionDetailLayout).toBeTypeOf(
      "function",
    );
    expect(currentSelectionBasePublic.NoSelectionDetailCard).toBeTypeOf(
      "function",
    );
    expect(currentSelectionBasePublic.DetailCardFactorySaveFeedback).toBeTypeOf(
      "function",
    );
    expect(
      currentSelectionBasePublic.mergeDetailCardSaveFieldErrors,
    ).toBeTypeOf("function");
    expect(currentSelectionBasePublic.CurrentSelectionDetailSection).toBeTypeOf(
      "function",
    );
    expect(currentSelectionBasePublic.CurrentSelectionFormFields).toBeTypeOf(
      "object",
    );
    expect(currentSelectionBasePublic.EditableConfigurationSaveRow).toBeTypeOf(
      "function",
    );
    expect(
      currentSelectionBasePublic.EditableConfigurationDiscardHeaderAction,
    ).toBeTypeOf("function");
    expect(
      currentSelectionBasePublic.getEditableConfigurationControlsMessages,
    ).toBeTypeOf("function");
  });
});

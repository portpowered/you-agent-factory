import { describe, expect, it } from "vitest";

import * as currentSelectionBasePublic from "./index";

describe("current-selection base public barrel", () => {
  it("exports shared selection contracts and layout primitives", () => {
    expect(currentSelectionBasePublic.resolveDashboardSelection).toBeTypeOf("function");
    expect(currentSelectionBasePublic.selectDefaultSelection).toBeTypeOf("function");
    expect(currentSelectionBasePublic.resetSelectionHistoryStore).toBeTypeOf("function");
    expect(currentSelectionBasePublic.SelectionDetailLayout).toBeTypeOf("function");
    expect(currentSelectionBasePublic.NoSelectionDetailCard).toBeTypeOf("function");
    expect(currentSelectionBasePublic.DetailCardFactorySaveFeedback).toBeTypeOf("function");
    expect(currentSelectionBasePublic.mergeDetailCardSaveFieldErrors).toBeTypeOf("function");
    expect(currentSelectionBasePublic.PROVIDER_SESSION_CARD_CLASS).toBeTypeOf("string");
    expect(currentSelectionBasePublic.CURRENT_SELECTION_VERTICAL_FORM_FIELDS_CLASS).toBe(
      "grid grid-cols-1 gap-3",
    );
    expect(currentSelectionBasePublic.EditableConfigurationSaveRow).toBeTypeOf("function");
  });
});

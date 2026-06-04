import {
  CURRENT_SELECTION_EXPANDABLE_SECTION_BODY_CLASS,
  CURRENT_SELECTION_FIELD_PANEL_CLASS,
  CURRENT_SELECTION_FORM_FIELD_CLASS,
  HISTORY_HEADER_CLASS,
  WORKSTATION_SUMMARY_ITEM_CLASS,
} from "./detail-card-shared";

describe("detail-card-shared row surfaces", () => {
  it("uses raised default surfaces for shared nested detail rows", () => {
    for (const className of [
      HISTORY_HEADER_CLASS,
      WORKSTATION_SUMMARY_ITEM_CLASS,
      CURRENT_SELECTION_FIELD_PANEL_CLASS,
      CURRENT_SELECTION_EXPANDABLE_SECTION_BODY_CLASS,
    ]) {
      expect(className).toContain("bg-surface-container-high");
      expect(className).not.toContain("bg-surface-container-low");
    }
  });

  it("keeps expandable section form fields free of per-field outlines", () => {
    expect(CURRENT_SELECTION_EXPANDABLE_SECTION_BODY_CLASS).toContain("border");
    expect(CURRENT_SELECTION_FORM_FIELD_CLASS).not.toContain("border");
  });
});

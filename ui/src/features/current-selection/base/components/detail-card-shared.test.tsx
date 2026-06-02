import {
  CURRENT_SELECTION_FIELD_PANEL_CLASS,
  HISTORY_HEADER_CLASS,
  PROVIDER_SESSION_CARD_CLASS,
  WORKSTATION_SUMMARY_ITEM_CLASS,
} from "./detail-card-shared";

describe("detail-card-shared row surfaces", () => {
  it("uses raised default surfaces for shared nested detail rows", () => {
    for (const className of [
      HISTORY_HEADER_CLASS,
      WORKSTATION_SUMMARY_ITEM_CLASS,
      CURRENT_SELECTION_FIELD_PANEL_CLASS,
    ]) {
      expect(className).toContain("bg-af-surface-raised");
      expect(className).not.toContain("bg-af-surface-subtle");
    }
  });

  it("keeps provider session cards on the subtle surface", () => {
    expect(PROVIDER_SESSION_CARD_CLASS).toContain("bg-af-surface-subtle");
    expect(PROVIDER_SESSION_CARD_CLASS).not.toContain("bg-af-surface-raised");
  });
});

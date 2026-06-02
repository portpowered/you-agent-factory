import { buildCurrentSelectionSaveToastMessages } from "./build-current-selection-save-toast-messages";

describe("buildCurrentSelectionSaveToastMessages", () => {
  it("includes the renamed workstation name in workstation save success copy", () => {
    const messages = buildCurrentSelectionSaveToastMessages({
      entityDisplayName: "Senior Review",
      entityKind: "workstation",
      locale: "en",
    });

    expect(messages.saveSuccessDescription).toBe(
      "Running factory saved. Senior Review was updated in the running factory definition.",
    );
  });
});

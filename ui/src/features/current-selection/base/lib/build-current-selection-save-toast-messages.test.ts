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

  it("includes the doc display label in doc save success copy", () => {
    const messages = buildCurrentSelectionSaveToastMessages({
      entityDisplayName: "playbook.md",
      entityKind: "doc",
      locale: "en",
    });

    expect(messages.saveSuccessDescription).toBe(
      "Running factory saved. playbook.md was updated in the running factory definition.",
    );
    expect(messages.saveFailedTitle).toBe("Doc save failed");
  });
});

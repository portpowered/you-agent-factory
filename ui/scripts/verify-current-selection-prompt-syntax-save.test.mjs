import { describe, expect, test, vi } from "vitest";

import { verifyCurrentSelectionPromptSyntaxSave } from "./verify-current-selection-storybook-responsive.mjs";

function createVisibleLocator(label, overrides = {}) {
  return {
    click: vi.fn().mockResolvedValue(undefined),
    count: vi.fn().mockResolvedValue(1),
    evaluate: vi.fn().mockResolvedValue(undefined),
    isDisabled: vi.fn().mockResolvedValue(false),
    isVisible: vi.fn().mockResolvedValue(true),
    selectOption: vi.fn().mockResolvedValue(undefined),
    ...overrides,
    label,
  };
}

function createPromptSyntaxSaveFixture() {
  const expandButton = createVisibleLocator("Expand editable configuration");
  expandButton.click = vi.fn().mockResolvedValue(undefined);
  const promptField = createVisibleLocator("Prompt textbox");
  const saveButton = createVisibleLocator("Save changes button", {
    isDisabled: vi.fn().mockResolvedValueOnce(true).mockResolvedValue(false),
  });
  saveButton.click = vi.fn().mockResolvedValue(undefined);
  const summary = createVisibleLocator("Prompt validation summary");
  const lineDiagnostic = createVisibleLocator("Line-based syntax diagnostic");
  const confirmationDialog = createVisibleLocator("Confirmation dialog");
  const squiggle = createVisibleLocator("Monaco syntax squiggle", {
    count: vi.fn().mockResolvedValue(1),
  });
  const currentSelection = {
    getByRole: vi.fn((role, options) => {
      if (
        role === "button" &&
        options?.name === "Expand editable configuration"
      ) {
        return expandButton;
      }
      if (role === "textbox" && options?.name === "Prompt") {
        return promptField;
      }
      if (role === "button" && options?.name === "Save changes") {
        return saveButton;
      }
      return createVisibleLocator(`${role}:${options?.name}`);
    }),
    getByText: vi.fn((text) => {
      if (text === "Fix highlighted issues before saving.") {
        return summary;
      }
      if (text === "Validating prompt variables for the current draft.") {
        return createVisibleLocator("Prompt validation status", {
          count: vi.fn().mockResolvedValue(0),
        });
      }
      if (text instanceof RegExp && text.source === "line 1:") {
        return lineDiagnostic;
      }
      return createVisibleLocator(`text:${String(text)}`);
    }),
    waitFor: vi.fn().mockResolvedValue(undefined),
  };
  const page = {
    evaluate: vi.fn().mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
    getByRole: vi.fn((role, options) => {
      if (role === "article" && options?.name === "Current selection") {
        return currentSelection;
      }
      if (
        role === "dialog" &&
        options?.name === "Overwrite the running factory definition?"
      ) {
        return confirmationDialog;
      }
      return createVisibleLocator(`${role}:${options?.name}`);
    }),
    keyboard: {
      insertText: vi.fn().mockResolvedValue(undefined),
      press: vi.fn().mockResolvedValue(undefined),
    },
    locator: vi.fn((selector) => {
      if (
        selector ===
        '[data-monaco-editor="workstation-prompt"] .squiggly-error'
      ) {
        return squiggle;
      }
      if (
        selector ===
        '[data-monaco-editor="workstation-prompt"] .native-edit-context'
      ) {
        return createVisibleLocator("Prompt editor focus target");
      }
      return createVisibleLocator(selector);
    }),
  };

  summary.count = vi.fn().mockResolvedValue(1);

  return {
    confirmationDialog,
    expandButton,
    lineDiagnostic,
    page,
    saveButton,
    squiggle,
    summary,
  };
}

describe("verifyCurrentSelectionPromptSyntaxSave", () => {
  test("verifies syntax diagnostics, Monaco squiggles, save gating, and confirmation", async () => {
    const {
      confirmationDialog,
      expandButton,
      lineDiagnostic,
      page,
      saveButton,
      squiggle,
      summary,
    } = createPromptSyntaxSaveFixture();
    const expectVisible = vi.fn((_locator) => Promise.resolve());
    const expectNoHorizontalOverflow = vi.fn().mockResolvedValue(undefined);

    await verifyCurrentSelectionPromptSyntaxSave({
      expectNoHorizontalOverflow,
      expectVisible,
      page,
      viewport: { height: 900, label: "desktop", width: 1440 },
    });

    expect(expandButton.click).toHaveBeenCalled();
    expect(page.keyboard.press).toHaveBeenCalledWith("ControlOrMeta+A");
    expect(page.keyboard.insertText).toHaveBeenCalledWith("{{ if .WorkID }}");
    expect(page.keyboard.insertText).toHaveBeenCalledWith(
      "{{ if .WorkID }}{{ end }}",
    );
    expect(summary.count).toHaveBeenCalled();
    expect(squiggle.count).toHaveBeenCalled();
    expect(saveButton.isDisabled).toHaveBeenCalled();
    expect(saveButton.click).toHaveBeenCalled();
    expect(expectVisible).toHaveBeenCalledWith(
      summary,
      "Prompt validation blocking summary",
    );
    expect(expectVisible).toHaveBeenCalledWith(
      lineDiagnostic,
      "Line-based syntax diagnostic",
    );
    expect(expectVisible).toHaveBeenCalledWith(
      confirmationDialog,
      "Editable workstation save confirmation dialog",
    );
    expect(expectNoHorizontalOverflow).toHaveBeenCalledWith(
      page,
      "Current selection prompt syntax save at desktop",
    );
  });
});

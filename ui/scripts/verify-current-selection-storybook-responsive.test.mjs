import { describe, expect, test, vi } from "vitest";

import {
  verifyCurrentSelectionPromptHint,
  verifyCurrentSelectionSaveFlow,
} from "./verify-current-selection-storybook-responsive.mjs";

function createVisibleLocator(label, overrides = {}) {
  return {
    count: vi.fn().mockResolvedValue(1),
    click: vi.fn().mockResolvedValue(undefined),
    evaluate: vi.fn().mockResolvedValue(undefined),
    fill: vi.fn().mockResolvedValue(undefined),
    first: vi.fn(function first() {
      return this;
    }),
    filter: vi.fn(function filter() {
      return this;
    }),
    last: vi.fn(function last() {
      return this;
    }),
    focus: vi.fn().mockResolvedValue(undefined),
    isDisabled: vi.fn().mockResolvedValue(false),
    isVisible: vi.fn().mockResolvedValue(true),
    label,
    locator: vi.fn(function locator() {
      return this;
    }),
    waitFor: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

function createSaveFlowHarness() {
  const expandButton = createVisibleLocator("Expand editable configuration");
  expandButton.click = vi.fn().mockResolvedValue(undefined);
  const promptField = createVisibleLocator("Prompt textbox");
  promptField.click = vi.fn().mockResolvedValue(undefined);
  const workerField = createVisibleLocator("Worker combobox");
  workerField.selectOption = vi.fn().mockResolvedValue(undefined);
  const saveButton = createVisibleLocator("Save changes button", {
    isDisabled: vi
      .fn()
      .mockResolvedValueOnce(true)
      .mockResolvedValueOnce(false)
      .mockResolvedValueOnce(true),
  });
  saveButton.click = vi.fn().mockResolvedValue(undefined);
  const successMessage = createVisibleLocator("Save success message");
  const confirmationDialog = createVisibleLocator("Confirmation dialog", {
    count: vi.fn().mockResolvedValue(0),
  });
  const overwriteButton = createVisibleLocator("Overwrite factory button");
  overwriteButton.click = vi.fn().mockResolvedValue(undefined);
  confirmationDialog.getByRole = vi.fn((role, options) => {
    if (role === "button" && options?.name === "Overwrite factory") {
      return overwriteButton;
    }
    return createVisibleLocator(`${role}:${options?.name}`);
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
      if (role === "combobox" && options?.name === "Worker") {
        return workerField;
      }
      if (role === "button" && options?.name === "Save changes") {
        return saveButton;
      }
      return createVisibleLocator(`${role}:${options?.name}`);
    }),
    getByText: vi.fn((text) => {
      if (
        text ===
        "Running factory saved. The editable workstation values were refreshed to the saved definition."
      ) {
        return successMessage;
      }
      if (text === "Validating prompt variables for the current draft.") {
        return createVisibleLocator("Prompt validation status", {
          count: vi.fn().mockResolvedValue(0),
        });
      }
      return createVisibleLocator(`text:${text}`);
    }),
    isVisible: vi.fn().mockResolvedValue(true),
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
      type: vi.fn().mockResolvedValue(undefined),
    },
  };

  return {
    confirmationDialog,
    expandButton,
    overwriteButton,
    page,
    promptField,
    saveButton,
    successMessage,
    workerField,
  };
}

function createPromptHintFixture() {
  const calls = [];
  const planWorkstationButton = createVisibleLocator("Select Plan workstation");
  planWorkstationButton.click = vi.fn().mockResolvedValue(undefined);
  const implementWorkstationButton = createVisibleLocator(
    "Select Implement workstation",
  );
  implementWorkstationButton.click = vi.fn().mockResolvedValue(undefined);
  const reviewWorkstationButton = createVisibleLocator(
    "Select Review workstation",
  );
  reviewWorkstationButton.click = vi.fn().mockResolvedValue(undefined);
  const expandEditableConfigurationButton = createVisibleLocator(
    "Expand editable configuration",
  );
  const removedHelpButton = createVisibleLocator("removed help button", {
    count: vi.fn().mockResolvedValue(0),
  });
  const legacySquiggleOverlay = createVisibleLocator("legacy mark", {
    count: vi.fn().mockResolvedValue(0),
  });
  const promptField = createVisibleLocator("Prompt textbox");
  promptField.click = vi.fn().mockResolvedValue(undefined);
  const suggestionWidget = createVisibleLocator("Prompt suggestion widget");
  const completedPromptLine = createVisibleLocator("Completed prompt line");
  const saveButton = createVisibleLocator("Save changes button", {
    isDisabled: vi.fn().mockResolvedValue(true),
  });
  const currentSelection = {
    getByRole: vi.fn((role, options) => {
      calls.push(["role", role, options]);
      if (role === "textbox" && options?.name === "Prompt") {
        return promptField;
      }
      if (
        role === "button" &&
        options?.name === "Expand editable configuration"
      ) {
        return expandEditableConfigurationButton;
      }
      if (role === "button" && options?.name === "Save changes") {
        return saveButton;
      }
      if (
        role === "button" &&
        String(options?.name).includes("prompt variable help")
      ) {
        return removedHelpButton;
      }
      return createVisibleLocator(`${role}:${options?.name}`);
    }),
    getByText: vi.fn((text) => {
      calls.push(["text", text]);
      return createVisibleLocator(`text:${text}`);
    }),
    locator: vi.fn((selector) => {
      calls.push(["locator", selector]);
      if (selector === "mark") {
        return legacySquiggleOverlay;
      }
      return createVisibleLocator(`locator:${selector}`);
    }),
    waitFor: vi.fn().mockResolvedValue(undefined),
  };
  const page = {
    evaluate: vi.fn().mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
    getByRole: vi.fn((role, options) => {
      if (role === "article" && options?.name === "Current selection") {
        return currentSelection;
      }
      if (role === "button" && options?.name === "Select Plan workstation") {
        return planWorkstationButton;
      }
      if (
        role === "button" &&
        options?.name === "Select Implement workstation"
      ) {
        return implementWorkstationButton;
      }
      if (role === "button" && options?.name === "Select Review workstation") {
        return reviewWorkstationButton;
      }
      return createVisibleLocator(`${role}:${options?.name}`);
    }),
    keyboard: {
      press: vi.fn().mockResolvedValue(undefined),
      type: vi.fn().mockResolvedValue(undefined),
    },
    locator: vi.fn((selector) => {
      if (selector === ".suggest-widget.visible") {
        return suggestionWidget;
      }
      if (selector === '[data-monaco-editor="workstation-prompt"] .view-line') {
        return completedPromptLine;
      }
      return createVisibleLocator(`page-locator:${selector}`);
    }),
  };

  return {
    calls,
    currentSelection,
    expectNoHorizontalOverflow: vi.fn().mockResolvedValue(undefined),
    expectVisible: vi.fn((_locator) => Promise.resolve()),
    expandEditableConfigurationButton,
    implementWorkstationButton,
    legacySquiggleOverlay,
    page,
    planWorkstationButton,
    promptField,
    removedHelpButton,
    reviewWorkstationButton,
    saveButton,
    completedPromptLine,
    suggestionWidget,
  };
}

describe("verifyCurrentSelectionPromptHint", () => {
  test("expands editable configuration before verifying Monaco inline guidance", async () => {
    const {
      calls,
      currentSelection,
      expectNoHorizontalOverflow,
      expectVisible,
      expandEditableConfigurationButton,
      implementWorkstationButton,
      legacySquiggleOverlay,
      page,
      planWorkstationButton,
      promptField,
      removedHelpButton,
      reviewWorkstationButton,
      saveButton,
      completedPromptLine,
      suggestionWidget,
    } = createPromptHintFixture();

    await verifyCurrentSelectionPromptHint({
      expectNoHorizontalOverflow,
      expectVisible,
      page,
      viewport: { height: 900, label: "desktop", width: 1440 },
    });

    expect(currentSelection.getByRole).not.toHaveBeenCalledWith("button", {
      name: "Close prompt variable help",
    });
    expect(removedHelpButton.count).toHaveBeenCalled();
    expect(legacySquiggleOverlay.count).toHaveBeenCalled();
    expect(planWorkstationButton.focus).toHaveBeenCalled();
    expect(implementWorkstationButton.focus).toHaveBeenCalled();
    expect(reviewWorkstationButton.focus).toHaveBeenCalled();
    expect(expandEditableConfigurationButton.focus).toHaveBeenCalled();
    expect(page.keyboard.press).toHaveBeenCalledWith("Enter");
    expect(page.locator).toHaveBeenCalledWith(
      '[data-monaco-editor="workstation-prompt"] .native-edit-context',
    );
    expect(expectVisible).toHaveBeenCalledWith(
      suggestionWidget,
      "Prompt autocomplete expanded input-field suggestion",
    );
    expect(expectVisible).toHaveBeenCalledWith(
      suggestionWidget,
      "Prompt autocomplete prefix-based input-field suggestion",
    );
    expect(expectVisible).toHaveBeenCalledWith(
      completedPromptLine,
      "Prompt editor accepted prefix-based input-field completion",
    );
    expect(page.keyboard.press).toHaveBeenCalledWith("ControlOrMeta+A");
    expect(page.keyboard.type).toHaveBeenCalledWith(
      "Use {{ (index .Inputs 0).",
    );
    expect(page.keyboard.type).toHaveBeenCalledWith(
      "Use {{ (index .Inputs 1).Payload }}.",
    );
    expect(suggestionWidget.click).toHaveBeenCalled();
    expect(saveButton.isDisabled).toHaveBeenCalled();
    expect(expectVisible).toHaveBeenCalledWith(promptField, "Prompt field");
    expect(expectNoHorizontalOverflow).toHaveBeenCalledWith(
      page,
      "Current selection prompt hinting at desktop",
    );
    expect(calls).toContainEqual([
      "text",
      "Save stays disabled until the prompt validates cleanly for this workstation context.",
    ]);
    expect(calls).toContainEqual([
      "role",
      "heading",
      { name: "Available variables" },
    ]);
    expect(calls).toContainEqual(["text", ".WorkID"]);
    expect(calls).toContainEqual([
      "role",
      "heading",
      { name: "Unavailable access" },
    ]);
  });
});

describe("verifyCurrentSelectionSaveFlow", () => {
  test("verifies the save confirmation, pending state, and refreshed success state", async () => {
    const {
      confirmationDialog,
      expandButton,
      overwriteButton,
      page,
      promptField,
      saveButton,
      successMessage,
      workerField,
    } = createSaveFlowHarness();
    const expectVisible = vi.fn((_locator) => Promise.resolve());
    const expectNoHorizontalOverflow = vi.fn().mockResolvedValue(undefined);

    await verifyCurrentSelectionSaveFlow({
      expectNoHorizontalOverflow,
      expectVisible,
      page,
      viewport: { height: 844, label: "mobile", width: 390 },
    });

    expect(expandButton.click).toHaveBeenCalled();
    expect(workerField.selectOption).toHaveBeenCalledWith("planner");
    expect(promptField.click).toHaveBeenCalledWith({ force: true });
    expect(page.keyboard.press).toHaveBeenCalledWith("ControlOrMeta+A");
    expect(page.keyboard.insertText).toHaveBeenCalledWith(
      "Browser verified prompt update.",
    );
    expect(saveButton.click).toHaveBeenCalled();
    expect(overwriteButton.click).toHaveBeenCalled();
    expect(expectVisible).toHaveBeenCalledWith(
      successMessage,
      "Editable workstation save success message",
    );
    expect(confirmationDialog.count).toHaveBeenCalled();
    expect(saveButton.isDisabled).toHaveBeenCalled();
    expect(expectNoHorizontalOverflow).toHaveBeenCalledWith(
      page,
      "Current selection save flow at mobile",
    );
  });
});

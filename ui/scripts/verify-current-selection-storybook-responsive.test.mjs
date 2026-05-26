import { describe, expect, test, vi } from "vitest";

import {
  verifyCurrentSelectionPromptHint,
  verifyCurrentSelectionSaveFlow,
  verifyCurrentSelectionWorkstationDetailOrder,
} from "./verify-current-selection-storybook-responsive.mjs";

function createVisibleLocator(label, overrides = {}) {
  return {
    count: vi.fn().mockResolvedValue(1),
    evaluate: vi.fn().mockResolvedValue(undefined),
    fill: vi.fn().mockResolvedValue(undefined),
    first: vi.fn(function first() {
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
      if (role === "button" && options?.name === "Expand editable configuration") {
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
    evaluate: vi
      .fn()
      .mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
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

describe("verifyCurrentSelectionPromptHint", () => {
  test("expands editable configuration before verifying Monaco inline guidance", async () => {
    const calls = [];
    const planWorkstationButton = createVisibleLocator("Select Plan workstation");
    planWorkstationButton.click = vi.fn().mockResolvedValue(undefined);
    const implementWorkstationButton = createVisibleLocator(
      "Select Implement workstation",
    );
    implementWorkstationButton.click = vi.fn().mockResolvedValue(undefined);
    const reviewWorkstationButton = createVisibleLocator("Select Review workstation");
    reviewWorkstationButton.click = vi.fn().mockResolvedValue(undefined);
    const expandEditableConfigurationButton = createVisibleLocator(
      "Expand editable configuration",
    );
    const removedHelpButton = createVisibleLocator("removed help button", {
      count: vi.fn().mockResolvedValue(0),
    });
    const removedHelpPanel = createVisibleLocator("removed help panel", {
      count: vi.fn().mockResolvedValue(0),
    });
    const legacySquiggleOverlay = createVisibleLocator("legacy mark", {
      count: vi.fn().mockResolvedValue(0),
    });
    const promptField = createVisibleLocator("Prompt textbox");
    promptField.click = vi.fn().mockResolvedValue(undefined);
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
        if (role === "button" && String(options?.name).includes("prompt variable help")) {
          return removedHelpButton;
        }
        return createVisibleLocator(`${role}:${options?.name}`);
      }),
      getByText: vi.fn((text) => {
        calls.push(["text", text]);
        if (text === "Available variables") {
          return removedHelpPanel;
        }
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
      evaluate: vi
        .fn()
        .mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
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
    };
    const expectVisible = vi.fn((_locator) => Promise.resolve());
    const expectNoHorizontalOverflow = vi.fn().mockResolvedValue(undefined);

    await verifyCurrentSelectionPromptHint({
      expectNoHorizontalOverflow,
      expectVisible,
      page,
      viewport: { height: 844, label: "mobile", width: 390 },
    });

    expect(currentSelection.getByRole).not.toHaveBeenCalledWith("button", {
      name: "Close prompt variable help",
    });
    expect(removedHelpButton.count).toHaveBeenCalled();
    expect(removedHelpPanel.count).toHaveBeenCalled();
    expect(legacySquiggleOverlay.count).toHaveBeenCalled();
    expect(planWorkstationButton.focus).toHaveBeenCalled();
    expect(implementWorkstationButton.focus).toHaveBeenCalled();
    expect(reviewWorkstationButton.focus).toHaveBeenCalled();
    expect(expandEditableConfigurationButton.focus).toHaveBeenCalled();
    expect(page.keyboard.press).toHaveBeenCalledWith("Enter");
    expect(promptField.click).toHaveBeenCalledWith({ force: true });
    expect(page.keyboard.press).toHaveBeenCalledWith("ControlOrMeta+A");
    expect(page.keyboard.type).toHaveBeenCalledWith(
      "Use {{ (index .Inputs 1).Payload }}.",
    );
    expect(saveButton.isDisabled).toHaveBeenCalled();
    expect(expectVisible).toHaveBeenCalledWith(
      promptField,
      "Prompt field",
    );
    expect(expectNoHorizontalOverflow).toHaveBeenCalledWith(
      page,
      "Current selection prompt hinting at mobile",
    );
    expect(calls).toContainEqual([
      "text",
      "Save stays disabled until the prompt validates cleanly for this workstation context.",
    ]);
  });
});

describe("verifyCurrentSelectionWorkstationDetailOrder", () => {
  test("verifies the reordered workstation detail sections and request-history access", async () => {
    const summaryHeading = createVisibleLocator("Workstation summary", {
      boundingBox: vi.fn().mockResolvedValue({ top: 10 }),
    });
    const configurationHeading = createVisibleLocator("Configuration", {
      boundingBox: vi.fn().mockResolvedValue({ top: 60 }),
    });
    const activeWorkHeading = createVisibleLocator("Active work", {
      boundingBox: vi.fn().mockResolvedValue({ top: 110 }),
    });
    const requestHistoryHeading = createVisibleLocator("Request history", {
      count: vi.fn().mockResolvedValue(0),
      boundingBox: vi.fn().mockResolvedValue({ top: 160 }),
    });
    const runHistoryHeading = createVisibleLocator("Run history", {
      boundingBox: vi.fn().mockResolvedValue({ top: 160 }),
    });
    const expandButton = createVisibleLocator("Expand button");
    expandButton.click = vi.fn().mockResolvedValue(undefined);
    const activeWorkButton = createVisibleLocator("Active Story button");
    const rejectedStoryButton = createVisibleLocator("Rejected Story button");
    const currentSelection = {
      getByRole: vi.fn((role, options) => {
        if (role === "heading" && options?.name === "Workstation summary") {
          return summaryHeading;
        }
        if (role === "heading" && options?.name === "Configuration") {
          return configurationHeading;
        }
        if (role === "heading" && options?.name === "Active work") {
          return activeWorkHeading;
        }
        if (role === "heading" && options?.name === "Request history") {
          return requestHistoryHeading;
        }
        if (role === "heading" && options?.name === "Run history") {
          return runHistoryHeading;
        }
        if (
          role === "button" &&
          options?.name === "Select work item Active Story"
        ) {
          return activeWorkButton;
        }
        if (role === "button" && options?.name === "Expand") {
          return expandButton;
        }
        if (
          role === "button" &&
          options?.name ===
            "Select provider session codex / Session ID / sess-rejected-story for dispatch dispatch-review-rejected"
        ) {
          return rejectedStoryButton;
        }
        return createVisibleLocator(`${role}:${options?.name}`);
      }),
      getByText: vi.fn((text) => createVisibleLocator(`text:${text}`)),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const page = {
      getByRole: vi.fn(() => currentSelection),
    };
    const expectVisible = vi.fn((_locator) => Promise.resolve());
    const expectNoHorizontalOverflow = vi.fn().mockResolvedValue(undefined);

    await verifyCurrentSelectionWorkstationDetailOrder({
      expectNoHorizontalOverflow,
      expectVisible,
      page,
      viewport: { height: 844, label: "mobile", width: 390 },
    });

    expect(expectVisible).toHaveBeenCalledWith(
      summaryHeading,
      "Workstation summary heading",
    );
    expect(expectVisible).toHaveBeenCalledWith(
      expect.objectContaining({ label: "text:Input work types" }),
      "Workstation summary work-type label",
    );
    expect(expectVisible).toHaveBeenCalledWith(
      expect.objectContaining({ label: "text:Active runs" }),
      "Workstation summary activity-count label",
    );
    expect(expectVisible).toHaveBeenCalledWith(
      activeWorkButton,
      "Active work selection button",
    );
    expect(expandButton.click).toHaveBeenCalled();
    expect(expectVisible).toHaveBeenCalledWith(
      rejectedStoryButton,
      "History selection button",
    );
    expect(expectNoHorizontalOverflow).toHaveBeenCalledWith(
      page,
      "Current selection workstation detail order at mobile",
    );
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

import { describe, expect, test, vi } from "vitest";

import { verifyCurrentSelectionPromptHint } from "./verify-current-selection-storybook-responsive.mjs";

function createVisibleLocator(label, overrides = {}) {
  return {
    count: vi.fn().mockResolvedValue(1),
    fill: vi.fn().mockResolvedValue(undefined),
    first: vi.fn(function first() {
      return this;
    }),
    focus: vi.fn().mockResolvedValue(undefined),
    isDisabled: vi.fn().mockResolvedValue(false),
    isVisible: vi.fn().mockResolvedValue(true),
    label,
    waitFor: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

describe("verifyCurrentSelectionPromptHint", () => {
  test("verifies Monaco inline prompt guidance instead of the removed help disclosure", async () => {
    const calls = [];
    const removedHelpButton = createVisibleLocator("removed help button", {
      count: vi.fn().mockResolvedValue(0),
    });
    const removedHelpPanel = createVisibleLocator("removed help panel", {
      count: vi.fn().mockResolvedValue(0),
    });
    const legacySquiggleOverlay = createVisibleLocator("legacy mark", {
      count: vi.fn().mockResolvedValue(0),
    });
    const promptEditor = createVisibleLocator("Monaco prompt editor");
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
        return promptEditor;
      }),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const page = {
      evaluate: vi
        .fn()
        .mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
      getByRole: vi.fn(() => currentSelection),
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
    expect(promptField.click).toHaveBeenCalledWith({ force: true });
    expect(page.keyboard.press).toHaveBeenCalledWith("ControlOrMeta+A");
    expect(page.keyboard.type).toHaveBeenCalledWith(
      "Use {{ (index .Inputs 1).Payload }}.",
    );
    expect(saveButton.isDisabled).toHaveBeenCalled();
    expect(expectVisible).toHaveBeenCalledWith(
      promptEditor,
      "Monaco prompt editor",
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

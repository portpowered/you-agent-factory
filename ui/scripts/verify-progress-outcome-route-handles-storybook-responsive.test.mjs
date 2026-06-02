import { describe, expect, test, vi } from "vitest";

import {
  verifyProgressOutcomeRoutesWithoutStopWords,
  verifyProgressOutcomeRoutesWithStopWords,
} from "./verify-progress-outcome-route-handles-storybook-responsive.mjs";

function createButtonLocator(name, visible = true) {
  return {
    count: vi.fn().mockResolvedValue(visible ? 1 : 0),
    label: name,
  };
}

function createHintLocator(count) {
  return {
    count: vi.fn().mockResolvedValue(count),
  };
}

function createPage(buttons, hintCount = 0) {
  const hints = createHintLocator(hintCount);
  const continueHint = createHintLocator(hintCount > 0 ? 1 : 0);
  const rejectHint = createHintLocator(hintCount > 0 ? 1 : 0);

  return {
    getByRole: vi.fn((role, options) => {
      if (role !== "button") {
        throw new Error(`Unexpected role ${role}`);
      }
      const locator = buttons.get(String(options?.name));
      if (!locator) {
        throw new Error(`Unexpected button ${String(options?.name)}`);
      }
      return locator;
    }),
    locator: vi.fn((selector) => {
      if (selector === "[data-z-axis-incomplete-hint]") {
        return hints;
      }
      if (
        selector ===
          '[data-z-axis-incomplete-hint="workstation-on-continue-source"]' ||
        selector ===
          '[data-z-axis-incomplete-hint="workstation-on-rejection-source"]'
      ) {
        return selector.includes("continue") ? continueHint : rejectHint;
      }
      throw new Error(`Unexpected locator selector: ${selector}`);
    }),
  };
}

describe("verifyProgressOutcomeRouteHandlesStorybookResponsive", () => {
  test("requires success and failure handles without continue or reject", async () => {
    const buttons = new Map([
      ["Connect tool: draft Success", createButtonLocator("success")],
      ["Connect tool: draft Failure", createButtonLocator("failure")],
      ["Connect tool: draft Continue", createButtonLocator("continue", false)],
      ["Connect tool: draft Reject", createButtonLocator("reject", false)],
    ]);
    const expectVisible = vi.fn().mockResolvedValue(undefined);

    await verifyProgressOutcomeRoutesWithoutStopWords(
      { expectVisible },
      createPage(buttons, 0),
      { label: "desktop" },
    );

    expect(expectVisible).toHaveBeenCalledTimes(2);
  });

  test("fails when continue handles remain visible without stopWords", async () => {
    const buttons = new Map([
      ["Connect tool: draft Success", createButtonLocator("success")],
      ["Connect tool: draft Failure", createButtonLocator("failure")],
      ["Connect tool: draft Continue", createButtonLocator("continue")],
      ["Connect tool: draft Reject", createButtonLocator("reject", false)],
    ]);
    const expectVisible = vi.fn().mockResolvedValue(undefined);

    await expect(
      verifyProgressOutcomeRoutesWithoutStopWords(
        { expectVisible },
        createPage(buttons),
        { label: "desktop" },
      ),
    ).rejects.toThrow(/Continue handle should stay hidden/);
  });

  test("requires continue and reject handles when stopWords are configured", async () => {
    const buttons = new Map([
      ["Connect tool: draft Success", createButtonLocator("success")],
      ["Connect tool: draft Failure", createButtonLocator("failure")],
      ["Connect tool: draft Continue", createButtonLocator("continue")],
      ["Connect tool: draft Reject", createButtonLocator("reject")],
    ]);
    const expectVisible = vi.fn().mockResolvedValue(undefined);

    await verifyProgressOutcomeRoutesWithStopWords(
      { expectVisible },
      createPage(buttons, 0),
      { label: "desktop" },
    );

    expect(expectVisible).toHaveBeenCalledTimes(4);
  });

  test("fails when z-axis hints remain visible without stopWords", async () => {
    const buttons = new Map([
      ["Connect tool: draft Success", createButtonLocator("success")],
      ["Connect tool: draft Failure", createButtonLocator("failure")],
      ["Connect tool: draft Continue", createButtonLocator("continue", false)],
      ["Connect tool: draft Reject", createButtonLocator("reject", false)],
    ]);
    const expectVisible = vi.fn().mockResolvedValue(undefined);

    await expect(
      verifyProgressOutcomeRoutesWithoutStopWords(
        { expectVisible },
        createPage(buttons, 2),
        { label: "desktop" },
      ),
    ).rejects.toThrow(/Expected no z-axis incomplete hints/);
  });

  test("fails when z-axis hints remain visible with stopWords configured", async () => {
    const buttons = new Map([
      ["Connect tool: draft Success", createButtonLocator("success")],
      ["Connect tool: draft Failure", createButtonLocator("failure")],
      ["Connect tool: draft Continue", createButtonLocator("continue")],
      ["Connect tool: draft Reject", createButtonLocator("reject")],
    ]);
    const expectVisible = vi.fn().mockResolvedValue(undefined);

    await expect(
      verifyProgressOutcomeRoutesWithStopWords(
        { expectVisible },
        createPage(buttons, 2),
        { label: "desktop" },
      ),
    ).rejects.toThrow(/Z-axis incomplete hints should be absent/);
  });
});

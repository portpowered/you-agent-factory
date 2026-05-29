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

function createPage(buttons) {
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
  };
}

describe("verifyProgressOutcomeRouteHandlesStorybookResponsive", () => {
  test("requires success and failure handles without continue or reject", async () => {
    const buttons = new Map([
      ["Connect: draft Success", createButtonLocator("success")],
      ["Connect: draft Failure", createButtonLocator("failure")],
      ["Connect: draft Continue", createButtonLocator("continue", false)],
      ["Connect: draft Reject", createButtonLocator("reject", false)],
    ]);
    const expectVisible = vi.fn().mockResolvedValue(undefined);

    await verifyProgressOutcomeRoutesWithoutStopWords(
      { expectVisible },
      createPage(buttons),
      { label: "desktop" },
    );

    expect(expectVisible).toHaveBeenCalledTimes(2);
  });

  test("fails when continue handles remain visible without stopWords", async () => {
    const buttons = new Map([
      ["Connect: draft Success", createButtonLocator("success")],
      ["Connect: draft Failure", createButtonLocator("failure")],
      ["Connect: draft Continue", createButtonLocator("continue")],
      ["Connect: draft Reject", createButtonLocator("reject", false)],
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
      ["Connect: draft Success", createButtonLocator("success")],
      ["Connect: draft Failure", createButtonLocator("failure")],
      ["Connect: draft Continue", createButtonLocator("continue")],
      ["Connect: draft Reject", createButtonLocator("reject")],
    ]);
    const expectVisible = vi.fn().mockResolvedValue(undefined);

    await verifyProgressOutcomeRoutesWithStopWords(
      { expectVisible },
      createPage(buttons),
      { label: "desktop" },
    );

    expect(expectVisible).toHaveBeenCalledTimes(4);
  });
});

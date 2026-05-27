import { describe, expect, test, vi } from "vitest";

import { verifyCurrentSelectionWorkstationDetailOrder } from "./verify-current-selection-storybook-responsive.mjs";

function createVisibleLocator(label, overrides = {}) {
  return {
    boundingBox: vi.fn().mockResolvedValue({ top: 0 }),
    count: vi.fn().mockResolvedValue(1),
    first: vi.fn(function first() {
      return this;
    }),
    focus: vi.fn().mockResolvedValue(undefined),
    isVisible: vi.fn().mockResolvedValue(true),
    label,
    locator: vi.fn(function locator() {
      return this;
    }),
    waitFor: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

function createWorkstationDetailOrderHarness() {
  const selectReviewWorkstationButton = createVisibleLocator(
    "Select Review workstation",
  );
  selectReviewWorkstationButton.click = vi.fn().mockResolvedValue(undefined);
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
    boundingBox: vi.fn().mockResolvedValue({ top: 160 }),
    count: vi.fn().mockResolvedValue(0),
  });
  const runHistoryHeading = createVisibleLocator("Run history", {
    boundingBox: vi.fn().mockResolvedValue({ top: 160 }),
  });
  const runHistorySection = createVisibleLocator("Run history section");
  const expandButton = createVisibleLocator("Expand button");
  expandButton.click = vi.fn().mockResolvedValue(undefined);
  const activeWorkButton = createVisibleLocator("Active Story button");
  const rejectedStoryButton = createVisibleLocator("Rejected Story button");
  runHistoryHeading.locator = vi.fn((selector) =>
    selector === "xpath=ancestor::section[1]"
      ? runHistorySection
      : createVisibleLocator(`run-history-heading:${selector}`),
  );
  runHistorySection.getByRole = vi.fn((role, options) => {
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
  });
  const currentSelection = {
    count: vi.fn().mockResolvedValue(1),
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
      if (role === "button" && options?.name === "Select work item Active Story") {
        return activeWorkButton;
      }
      return createVisibleLocator(`${role}:${options?.name}`);
    }),
    getByText: vi.fn((text) => createVisibleLocator(`text:${text}`)),
    isVisible: vi.fn().mockResolvedValue(true),
    waitFor: vi.fn().mockResolvedValue(undefined),
  };
  const page = {
    getByRole: vi.fn((role, options) => {
      if (role === "button" && options?.name === "Select Review workstation") {
        return selectReviewWorkstationButton;
      }
      return currentSelection;
    }),
    keyboard: {
      press: vi.fn().mockResolvedValue(undefined),
    },
  };

  return {
    activeWorkButton,
    expandButton,
    page,
    rejectedStoryButton,
    runHistoryHeading,
    runHistorySection,
    selectReviewWorkstationButton,
    summaryHeading,
  };
}

describe("verifyCurrentSelectionWorkstationDetailOrder", () => {
  test("verifies reordered sections and request-history access", async () => {
    const harness = createWorkstationDetailOrderHarness();
    const expectVisible = vi.fn((_locator) => Promise.resolve());
    const expectNoHorizontalOverflow = vi.fn().mockResolvedValue(undefined);

    await verifyCurrentSelectionWorkstationDetailOrder({
      expectNoHorizontalOverflow,
      expectVisible,
      page: harness.page,
      viewport: { height: 844, label: "mobile", width: 390 },
    });

    expect(expectVisible).toHaveBeenCalledWith(
      harness.summaryHeading,
      "Workstation summary heading",
    );
    expect(harness.selectReviewWorkstationButton.click).not.toHaveBeenCalled();
    expect(expectVisible).toHaveBeenCalledWith(
      expect.objectContaining({ label: "text:Input work types" }),
      "Workstation summary work-type label",
    );
    expect(expectVisible).toHaveBeenCalledWith(
      expect.objectContaining({ label: "text:Active runs" }),
      "Workstation summary activity-count label",
    );
    expect(expectVisible).toHaveBeenCalledWith(
      harness.activeWorkButton,
      "Active work selection button",
    );
    expect(harness.runHistoryHeading.locator).toHaveBeenCalledWith(
      "xpath=ancestor::section[1]",
    );
    expect(harness.runHistorySection.getByRole).toHaveBeenCalledWith("button", {
      name: "Expand",
    });
    expect(harness.expandButton.focus).toHaveBeenCalled();
    expect(harness.page.keyboard.press).toHaveBeenCalledWith("Enter");
    expect(expectVisible).toHaveBeenCalledWith(
      harness.rejectedStoryButton,
      "History selection button",
    );
    expect(expectNoHorizontalOverflow).toHaveBeenCalledWith(
      harness.page,
      "Current selection workstation detail order at mobile",
    );
  });
});

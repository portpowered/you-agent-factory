import { describe, expect, test, vi } from "vitest";

import {
  expectMatchingDashboardShellStyles,
  verifyDashboardShellConsolidation,
} from "./dashboard-shell-storybook-responsive.mjs";

function matchingShellStyle() {
  return {
    backgroundColor: "rgba(255, 255, 255, 0.84)",
    borderBottomColor: "rgba(15, 23, 42, 0.1)",
    borderBottomLeftRadius: "8px",
    borderBottomRightRadius: "8px",
    borderBottomStyle: "solid",
    borderBottomWidth: "1px",
    borderLeftColor: "rgba(15, 23, 42, 0.1)",
    borderLeftStyle: "solid",
    borderLeftWidth: "1px",
    borderRightColor: "rgba(15, 23, 42, 0.1)",
    borderRightStyle: "solid",
    borderRightWidth: "1px",
    borderTopColor: "rgba(15, 23, 42, 0.1)",
    borderTopLeftRadius: "8px",
    borderTopRightRadius: "8px",
    borderTopStyle: "solid",
    borderTopWidth: "1px",
    boxShadow: "rgba(15, 23, 42, 0.08) 0px 8px 24px",
  };
}

describe("expectMatchingDashboardShellStyles", () => {
  test("accepts matching computed shell styles", async () => {
    const shellStyle = matchingShellStyle();
    const header = {
      evaluate: vi.fn().mockResolvedValue(shellStyle),
    };
    const gridCard = {
      evaluate: vi.fn().mockResolvedValue({ ...shellStyle }),
    };

    await expect(
      expectMatchingDashboardShellStyles(header, gridCard, "Dashboard shell"),
    ).resolves.toBeUndefined();
  });

  test("rejects mismatched computed shell styles", async () => {
    const header = {
      evaluate: vi.fn().mockResolvedValue({
        backgroundColor: "rgba(255, 255, 255, 0.84)",
        borderTopLeftRadius: "8px",
      }),
    };
    const gridCard = {
      evaluate: vi.fn().mockResolvedValue({
        backgroundColor: "rgb(255, 255, 255)",
        borderTopLeftRadius: "8px",
      }),
    };

    await expect(
      expectMatchingDashboardShellStyles(header, gridCard, "Dashboard shell"),
    ).rejects.toThrow(
      "Dashboard shell backgroundColor differed: header=rgba(255, 255, 255, 0.84), gridCard=rgb(255, 255, 255).",
    );
  });
});

describe("verifyDashboardShellConsolidation", () => {
  test("compares header and grid-card shell styles while preserving controls", async () => {
    const shellStyle = matchingShellStyle();
    const exportButton = { isVisible: vi.fn().mockResolvedValue(true) };
    const currentButton = { isVisible: vi.fn().mockResolvedValue(false) };
    const timelineSlider = { isVisible: vi.fn().mockResolvedValue(true) };
    const timelineStatus = { isVisible: vi.fn().mockResolvedValue(true) };
    const moveButton = { isVisible: vi.fn().mockResolvedValue(true) };
    const retiredStreamStatus = { count: vi.fn().mockResolvedValue(0) };
    const workTotalsCard = {
      evaluate: vi.fn().mockResolvedValue({ ...shellStyle }),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const toolbar = {
      evaluate: vi.fn().mockResolvedValue(shellStyle),
      getByRole: vi.fn((role, options) => {
        if (role === "status") {
          return retiredStreamStatus;
        }
        if (role === "slider" && options?.name === "Timeline tick") {
          return timelineSlider;
        }
        if (options?.name === "Return to current tick") {
          return currentButton;
        }
        return exportButton;
      }),
      getByText: vi.fn(() => timelineStatus),
      isVisible: vi.fn().mockResolvedValue(true),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const board = {
      getByRole: vi.fn((role, options) => {
        if (role === "article" && options?.name === "Work totals") {
          return workTotalsCard;
        }
        return moveButton;
      }),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const page = {
      evaluate: vi
        .fn()
        .mockResolvedValue({ clientWidth: 1440, scrollWidth: 1440 }),
      getByRole: vi.fn((_role, options) => {
        if (options?.name === "you-agent-factory bento board") {
          return board;
        }
        return toolbar;
      }),
    };

    await verifyDashboardShellConsolidation(page, null, {
      height: 900,
      label: "desktop",
      width: 1440,
    });

    expect(page.getByRole).toHaveBeenCalledWith("region", {
      name: "dashboard summary",
    });
    expect(page.getByRole).toHaveBeenCalledWith("region", {
      name: "you-agent-factory bento board",
    });
    expect(board.getByRole).toHaveBeenCalledWith("article", {
      name: "Work totals",
    });
    expect(board.getByRole).toHaveBeenCalledWith("button", {
      exact: true,
      name: "Move Work totals",
    });
    expect(toolbar.getByRole).toHaveBeenCalledWith("button", {
      name: "Export PNG",
    });
    expect(toolbar.getByRole).toHaveBeenCalledWith("slider", {
      name: "Timeline tick",
    });
  });
});

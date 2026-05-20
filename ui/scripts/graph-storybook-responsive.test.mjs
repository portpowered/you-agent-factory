import { describe, expect, test, vi } from "vitest";

import {
  expectMatchingDashboardGraphStyles,
  verifyTraceFactoryGraphVisualConsistency,
} from "./graph-storybook-responsive.mjs";

describe("graph-storybook-responsive", () => {
  test("expectMatchingDashboardGraphStyles accepts matching control styles", async () => {
    const expectedStyle = {
      backgroundColor: "rgba(255, 255, 255, 0.88)",
      borderTopLeftRadius: "8px",
    };

    await expect(
      expectMatchingDashboardGraphStyles(
        expectedStyle,
        { ...expectedStyle },
        "Trace graph controls",
      ),
    ).resolves.toBeUndefined();
  });

  test("expectMatchingDashboardGraphStyles rejects mismatched control styles", async () => {
    await expect(
      expectMatchingDashboardGraphStyles(
        { backgroundColor: "rgba(255, 255, 255, 0.88)" },
        { backgroundColor: "rgb(255, 255, 255)" },
        "Trace graph controls",
      ),
    ).rejects.toThrow(
      "Trace graph controls backgroundColor differed: expected=rgba(255, 255, 255, 0.88), actual=rgb(255, 255, 255).",
    );
  });

  test("compares factory and trace graph control chrome across stories", async () => {
    const graphControlsStyle = {
      backgroundColor: "rgba(255, 255, 255, 0.88)",
      borderBottomColor: "rgba(15, 23, 42, 0.08)",
      borderBottomStyle: "solid",
      borderBottomWidth: "1px",
      borderLeftColor: "rgba(15, 23, 42, 0.08)",
      borderLeftStyle: "solid",
      borderLeftWidth: "1px",
      borderRightColor: "rgba(15, 23, 42, 0.08)",
      borderRightStyle: "solid",
      borderRightWidth: "1px",
      borderTopColor: "rgba(15, 23, 42, 0.08)",
      borderTopStyle: "solid",
      borderTopWidth: "1px",
      borderTopLeftRadius: "8px",
      borderTopRightRadius: "8px",
      boxShadow: "none",
    };
    const controlLocator = {
      evaluate: vi.fn().mockResolvedValue(graphControlsStyle),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const controlLocatorWrapper = {
      first: vi.fn().mockReturnValue(controlLocator),
    };
    const factoryViewport = {
      locator: vi.fn().mockReturnValue(controlLocatorWrapper),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const dispatchViewport = {
      getByText: vi.fn().mockReturnValue({
        waitFor: vi.fn().mockResolvedValue(undefined),
      }),
      locator: vi.fn().mockReturnValue(controlLocatorWrapper),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const relationViewport = {
      getByText: vi.fn().mockReturnValue({
        waitFor: vi.fn().mockResolvedValue(undefined),
      }),
      locator: vi.fn().mockReturnValue(controlLocatorWrapper),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const page = {
      evaluate: vi
        .fn()
        .mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
      getByLabel: vi.fn((label) => {
        if (label === "Dispatch relationship graph") {
          return dispatchViewport;
        }
        if (label === "Batch relation graph") {
          return relationViewport;
        }

        throw new Error(`Unexpected getByLabel call: ${label}`);
      }),
      getByRole: vi.fn((role, options) => {
        if (role === "region" && options?.name === "Work graph viewport") {
          return factoryViewport;
        }
        if (
          role === "region" &&
          options?.name === "Dispatch relationship graph"
        ) {
          return dispatchViewport;
        }
        if (role === "region" && options?.name === "Batch relation graph") {
          return relationViewport;
        }

        throw new Error(`Unexpected getByRole call: ${role} ${options?.name}`);
      }),
      getByText: vi.fn().mockReturnValue({
        waitFor: vi.fn().mockResolvedValue(undefined),
      }),
      goto: vi.fn().mockResolvedValue(undefined),
    };

    await verifyTraceFactoryGraphVisualConsistency({
      expectNoHorizontalOverflow: vi.fn().mockResolvedValue(undefined),
      expectVisible: vi.fn().mockResolvedValue(undefined),
      page,
      viewport: { height: 844, label: "mobile", width: 390 },
      waitForStoryRender: vi.fn().mockResolvedValue(undefined),
    });

    expect(page.getByRole).toHaveBeenCalledWith("region", {
      name: "Work graph viewport",
    });
    expect(page.getByLabel).toHaveBeenCalledWith("Dispatch relationship graph");
    expect(page.getByLabel).toHaveBeenCalledWith("Batch relation graph");
    expect(page.getByText).toHaveBeenCalledWith("Observe mode");
    expect(dispatchViewport.getByText).toHaveBeenCalledWith(
      'Out: story:"Reviewed Story"',
    );
    expect(relationViewport.getByText).toHaveBeenCalledWith("Repair story");
    expect(page.goto).toHaveBeenCalledWith(
      "http://127.0.0.1:6008/iframe.html?id=agent-factory-dashboard-trace-graph-surfaces--visual-consistency&viewMode=story",
      { waitUntil: "domcontentloaded" },
    );
  });
});

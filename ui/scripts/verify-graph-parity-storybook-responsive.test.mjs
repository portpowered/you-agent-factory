import { describe, expect, test, vi } from "vitest";

import { verifyEditorGraphParity } from "./verify-graph-parity-storybook-responsive.mjs";

function createVisibleLocator(label, overrides = {}) {
  return {
    isVisible: vi.fn().mockResolvedValue(true),
    label,
    waitFor: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

describe("verifyEditorGraphParity", () => {
  test("targets the review workstation title exactly so reviewer does not match", async () => {
    const reviewNode = createVisibleLocator("review workstation");
    const gpuNode = createVisibleLocator("gpu resource", {
      isVisible: vi.fn().mockResolvedValue(false),
    });
    const allPreset = createVisibleLocator("All preset");
    const workflowPreset = createVisibleLocator("Workflow preset", {
      click: vi.fn().mockResolvedValue(undefined),
    });
    const infrastructurePreset = createVisibleLocator("Infrastructure preset", {
      click: vi.fn().mockResolvedValue(undefined),
    });
    const visibilityPresets = {
      getByRole: vi.fn((role, options) => {
        if (role !== "button") {
          throw new Error(`Unexpected role ${role}`);
        }
        if (String(options?.name) === "/^All$/") {
          return allPreset;
        }
        if (String(options?.name) === "/^Workflow$/") {
          return workflowPreset;
        }
        if (String(options?.name) === "/^Infrastructure$/") {
          return infrastructurePreset;
        }
        throw new Error(`Unexpected preset ${String(options?.name)}`);
      }),
    };
    const page = {
      getByRole: vi.fn((role, options) => {
        if (
          role === "region" &&
          options?.name === "Factory graph visibility presets"
        ) {
          return visibilityPresets;
        }
        throw new Error(`Unexpected role query ${role}:${options?.name}`);
      }),
      getByTitle: vi.fn((value) => {
        if (String(value) === "/^review$/") {
          return reviewNode;
        }
        if (value === "gpu") {
          return gpuNode;
        }
        throw new Error(`Unexpected title query ${String(value)}`);
      }),
    };
    const expectNoHorizontalOverflow = vi.fn().mockResolvedValue(undefined);
    const expectVisible = vi.fn().mockResolvedValue(undefined);

    await verifyEditorGraphParity(
      { expectNoHorizontalOverflow, expectVisible },
      page,
      { height: 900, label: "desktop", width: 1440 },
    );

    expect(page.getByTitle).toHaveBeenCalledWith(/^review$/);
    expect(expectVisible).toHaveBeenCalledWith(
      reviewNode,
      "Editor workstation node",
    );
    expect(workflowPreset.click).toHaveBeenCalledTimes(1);
    expect(reviewNode.waitFor).toHaveBeenCalledWith({ state: "visible" });
    expect(expectNoHorizontalOverflow).toHaveBeenCalledWith(
      page,
      "Editor graph parity at desktop",
    );
  });
});

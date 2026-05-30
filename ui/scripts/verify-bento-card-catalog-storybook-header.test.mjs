import { describe, expect, test } from "vitest";

import {
  assertCardHeaderMetrics,
  collectCardHeaderMetrics,
  verifyBentoCardCatalogHeader,
} from "./verify-bento-card-catalog-storybook-header.mjs";

function createArticleLocator(metrics, { moveButtonCount = 0 } = {}) {
  return {
    locator: (selector) => {
      if (selector === "header" || selector === 'header[data-bento-drag-handle="true"]') {
        return {
          evaluate: async () => metrics,
          waitFor: async () => undefined,
        };
      }

      throw new Error(`Unexpected locator selector: ${selector}`);
    },
    getByRole: (role, options) => {
      if (role === "button" && options?.name?.startsWith("Move ")) {
        return {
          count: async () => moveButtonCount,
        };
      }

      return {
        waitFor: async () => undefined,
      };
    },
  };
}

describe("verifyBentoCardCatalogHeader", () => {
  test("assertCardHeaderMetrics rejects overlapping title and header action", () => {
    expect(() =>
      assertCardHeaderMetrics(
        "Submit work",
        {
          compactChrome: false,
          hasGrabCursor: true,
          hasHeaderDragHandle: true,
          headerRect: { height: 52, width: 300, x: 0, y: 0 },
          primaryActionRects: [
            {
              height: 32,
              label: "Remove Submit work widget from dashboard",
              width: 32,
              x: 0,
              y: 0,
            },
          ],
          titleRect: { height: 20, width: 120, x: 0, y: 0 },
          titleTag: "H3",
        },
        { compactChrome: false },
      ),
    ).toThrow(/overlapped header action/);
  });

  test("assertCardHeaderMetrics accepts separated title and header action regions", () => {
    expect(() =>
      assertCardHeaderMetrics(
        "Provider session",
        {
          compactChrome: false,
          hasGrabCursor: true,
          hasHeaderDragHandle: true,
          headerRect: { height: 52, width: 240, x: 0, y: 0 },
          primaryActionRects: [],
          titleRect: { height: 20, width: 120, x: 0, y: 0 },
          titleTag: "H3",
        },
        { compactChrome: false },
      ),
    ).not.toThrow();
  });

  test("verifyBentoCardCatalogHeader exercises each representative card", async () => {
    const visible = new Set();
    const page = {
      getByRole: (role, options) => {
        if (role === "region") {
          return {
            waitFor: async () => undefined,
          };
        }

        if (role === "article") {
          const metricsByName = {
            "Add widget": {
              compactChrome: false,
              hasGrabCursor: true,
              hasHeaderDragHandle: true,
              headerRect: { height: 52, width: 260, x: 0, y: 0 },
              primaryActionRects: [],
              titleRect: { height: 20, width: 120, x: 0, y: 0 },
              titleTag: "H3",
            },
            "Provider session": {
              compactChrome: false,
              hasGrabCursor: true,
              hasHeaderDragHandle: true,
              headerRect: { height: 52, width: 260, x: 0, y: 0 },
              primaryActionRects: [],
              titleRect: { height: 20, width: 120, x: 0, y: 0 },
              titleTag: "H3",
            },
            "Submit work": {
              compactChrome: false,
              hasGrabCursor: true,
              hasHeaderDragHandle: true,
              headerRect: { height: 52, width: 300, x: 0, y: 0 },
              primaryActionRects: [
                {
                  height: 32,
                  label: "Remove Submit work widget from dashboard",
                  width: 32,
                  x: 200,
                  y: 0,
                },
              ],
              titleRect: { height: 20, width: 120, x: 0, y: 0 },
              titleTag: "H3",
            },
            "Work totals": {
              compactChrome: true,
              hasGrabCursor: true,
              hasHeaderDragHandle: true,
              headerRect: { height: 44, width: 240, x: 0, y: 0 },
              primaryActionRects: [],
              titleRect: { height: 20, width: 120, x: 0, y: 0 },
              titleTag: "H3",
            },
          };

          return createArticleLocator(metricsByName[options.name]);
        }

        throw new Error(`Unexpected role lookup: ${role}`);
      },
    };

    await verifyBentoCardCatalogHeader({
      expectNoHorizontalOverflow: async () => undefined,
      expectVisible: async (_locator, label) => {
        visible.add(label);
      },
      page,
      viewport: { label: "desktop" },
    });

    expect(visible.has("Header consistency bento board")).toBe(true);
    expect(visible.has("Work totals bento card")).toBe(true);
    expect(visible.has("Work totals header drag handle")).toBe(true);
    expect(visible.has("Submit work primary header action")).toBe(true);
    expect(visible.has("Add widget header drag handle")).toBe(true);
  });

  test("collectCardHeaderMetrics reports missing header pieces", async () => {
    const article = {
      locator: () => ({
        evaluate: async () => ({ error: "Missing bento card title." }),
      }),
    };

    await expect(collectCardHeaderMetrics(article)).resolves.toEqual({
      error: "Missing bento card title.",
    });
  });
});

import { describe, expect, test } from "vitest";

import {
  assertCardHeaderMetrics,
  collectCardHeaderMetrics,
  verifyBentoCardCatalogHeader,
} from "./verify-bento-card-catalog-storybook-header.mjs";

function createArticleLocator(metrics) {
  return {
    locator: () => ({
      evaluate: async () => metrics,
    }),
    getByRole: () => ({
      waitFor: async () => undefined,
    }),
  };
}

describe("verifyBentoCardCatalogHeader", () => {
  test("assertCardHeaderMetrics rejects overlapping title and drag handle", () => {
    expect(() =>
      assertCardHeaderMetrics(
        "Work totals",
        {
          compactChrome: true,
          handleRect: { height: 40, width: 40, x: 40, y: 0 },
          headerRect: { height: 44, width: 200, x: 0, y: 0 },
          primaryActionRects: [],
          titleRect: { height: 20, width: 60, x: 0, y: 0 },
          titleTag: "H3",
        },
        { compactChrome: true },
      ),
    ).toThrow(/overlapped the drag handle/);
  });

  test("assertCardHeaderMetrics accepts separated title and handle regions", () => {
    expect(() =>
      assertCardHeaderMetrics(
        "Provider session",
        {
          compactChrome: false,
          handleRect: { height: 40, width: 40, x: 180, y: 0 },
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
              handleRect: { height: 40, width: 40, x: 220, y: 0 },
              headerRect: { height: 52, width: 260, x: 0, y: 0 },
              primaryActionRects: [],
              titleRect: { height: 20, width: 120, x: 0, y: 0 },
              titleTag: "H3",
            },
            "Provider session": {
              compactChrome: false,
              handleRect: { height: 40, width: 40, x: 220, y: 0 },
              headerRect: { height: 52, width: 260, x: 0, y: 0 },
              primaryActionRects: [],
              titleRect: { height: 20, width: 120, x: 0, y: 0 },
              titleTag: "H3",
            },
            "Submit work": {
              compactChrome: false,
              handleRect: { height: 40, width: 40, x: 260, y: 0 },
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
              handleRect: { height: 40, width: 40, x: 180, y: 0 },
              headerRect: { height: 44, width: 240, x: 0, y: 0 },
              primaryActionRects: [],
              titleRect: { height: 20, width: 120, x: 0, y: 0 },
              titleTag: "H3",
            },
          };

          return createArticleLocator(metricsByName[options.name]);
        }

        if (role === "button") {
          return {
            waitFor: async () => undefined,
          };
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
    expect(visible.has("Submit work primary header action")).toBe(true);
    expect(visible.has("Add widget move handle")).toBe(true);
  });

  test("collectCardHeaderMetrics reports missing header pieces", async () => {
    const article = {
      locator: () => ({
        evaluate: async () => ({ error: "Missing bento card title or drag handle." }),
      }),
    };

    await expect(collectCardHeaderMetrics(article)).resolves.toEqual({
      error: "Missing bento card title or drag handle.",
    });
  });
});

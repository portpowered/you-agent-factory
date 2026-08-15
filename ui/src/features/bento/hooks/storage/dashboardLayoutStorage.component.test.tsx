// @vitest-environment jsdom
import "../../../../testing/vitest-dom-capabilities.setup";

import {
  createDashboardLayoutScope,
  DASHBOARD_LAYOUT_STORAGE_KEY,
  DASHBOARD_LAYOUT_STORAGE_VERSION,
  DEFAULT_DASHBOARD_LAYOUT,
  getDashboardLayoutStorageKey,
} from "../dashboardLayoutSchema";
import {
  readStoredDashboardLayoutResult,
  writeStoredDashboardLayout,
} from "./dashboardLayoutStorage";

const scope = createDashboardLayoutScope("factory-storage", "session-storage");

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: storage failure cases stay together as one observable contract.
describe("dashboard layout storage diagnostics", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("reports malformed JSON and keeps the default layout usable", () => {
    window.localStorage.setItem(
      getDashboardLayoutStorageKey(scope),
      "{malformed",
    );

    const result = readStoredDashboardLayoutResult(scope);

    expect(result.layout).toEqual(DEFAULT_DASHBOARD_LAYOUT);
    expect(result.diagnostics).toContainEqual(
      expect.objectContaining({ code: "malformed-json", severity: "error" }),
    );
  });

  it("reports unsupported scoped envelopes without reading the legacy key", () => {
    window.localStorage.setItem(
      getDashboardLayoutStorageKey(scope),
      JSON.stringify({
        layout: DEFAULT_DASHBOARD_LAYOUT,
        schemaVersion: DASHBOARD_LAYOUT_STORAGE_VERSION + 1,
        scope,
      }),
    );
    window.localStorage.setItem(
      DASHBOARD_LAYOUT_STORAGE_KEY,
      JSON.stringify(
        DEFAULT_DASHBOARD_LAYOUT.map((item) =>
          item.widgetType === "work-graph" ? { ...item, h: 14 } : item,
        ),
      ),
    );

    const result = readStoredDashboardLayoutResult(scope);

    expect(result.layout).toEqual(DEFAULT_DASHBOARD_LAYOUT);
    expect(result.diagnostics).toContainEqual(
      expect.objectContaining({
        code: "unsupported-envelope",
        severity: "error",
      }),
    );
  });

  it("persists a repaired scoped projection and reports the repair classes", () => {
    window.localStorage.setItem(
      getDashboardLayoutStorageKey(scope),
      JSON.stringify({
        layout: [
          {
            h: 0,
            id: "<unsafe>",
            w: 999,
            widgetType: "work-outcome-chart",
            x: -1,
            y: -1,
          },
        ],
        schemaVersion: DASHBOARD_LAYOUT_STORAGE_VERSION,
        scope,
      }),
    );

    const result = readStoredDashboardLayoutResult(scope);
    const persisted = JSON.parse(
      window.localStorage.getItem(getDashboardLayoutStorageKey(scope)) ?? "{}",
    ) as { layout?: Array<{ id: string; w: number; x: number; y: number }> };

    expect(result.diagnostics.map((diagnostic) => diagnostic.code)).toEqual(
      expect.arrayContaining(["invalid-id", "invalid-size", "out-of-bounds"]),
    );
    expect(persisted.layout?.every((item) => !/[<>]/u.test(item.id))).toBe(
      true,
    );
    expect(
      persisted.layout?.every(
        (item) => item.w > 0 && item.x >= 0 && item.x + item.w <= 12,
      ),
    ).toBe(true);
  });

  it("distinguishes quota and generic write failures", () => {
    const setItem = vi
      .spyOn(window.localStorage, "setItem")
      .mockImplementation(() => {
        throw { name: "QuotaExceededError" };
      });

    const quotaResult = writeStoredDashboardLayout(
      DEFAULT_DASHBOARD_LAYOUT,
      scope,
    );

    expect(quotaResult.persisted).toBe(false);
    expect(quotaResult.diagnostics).toContainEqual(
      expect.objectContaining({ code: "storage-quota-exceeded" }),
    );

    setItem.mockImplementation(() => {
      throw new Error("storage unavailable for writes");
    });
    const genericResult = writeStoredDashboardLayout(
      DEFAULT_DASHBOARD_LAYOUT,
      scope,
    );

    expect(genericResult.persisted).toBe(false);
    expect(genericResult.diagnostics).toContainEqual(
      expect.objectContaining({ code: "storage-write-failed" }),
    );
    setItem.mockRestore();
  });

  it("reports a throwing browser storage read", () => {
    const getItem = vi
      .spyOn(window.localStorage, "getItem")
      .mockImplementation(() => {
        throw new Error("storage unavailable for reads");
      });

    const result = readStoredDashboardLayoutResult(scope);

    expect(result.layout).toEqual(DEFAULT_DASHBOARD_LAYOUT);
    expect(result.diagnostics).toContainEqual(
      expect.objectContaining({ code: "storage-read-failed" }),
    );
    getItem.mockRestore();
  });

  it("reports unavailable browser storage when the localStorage getter throws", () => {
    const localStorageGetter = vi
      .spyOn(window, "localStorage", "get")
      .mockImplementation(() => {
        throw new Error("storage is blocked");
      });

    const result = readStoredDashboardLayoutResult(scope);

    expect(result.layout).toEqual(DEFAULT_DASHBOARD_LAYOUT);
    expect(result.diagnostics).toContainEqual(
      expect.objectContaining({ code: "storage-unavailable" }),
    );
    localStorageGetter.mockRestore();
  });
});

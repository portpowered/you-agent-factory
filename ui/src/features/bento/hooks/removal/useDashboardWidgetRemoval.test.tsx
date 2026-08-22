import "../../../../testing/vitest-dom-capabilities.setup";

import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AgentBentoLayoutItem } from "../../components/agent-bento";
import {
  DASHBOARD_WIDGET_UNDO_TIMEOUT_MS,
  useDashboardWidgetRemoval,
} from "./useDashboardWidgetRemoval";

const layout: AgentBentoLayoutItem[] = [
  {
    h: 2,
    id: "work-totals::primary",
    minH: 1,
    minW: 1,
    w: 4,
    widgetType: "work-totals",
    x: 0,
    y: 0,
  },
  {
    h: 4,
    id: "work-outcome-chart::instance-1",
    minH: 4,
    minW: 4,
    w: 4,
    widgetType: "work-outcome-chart",
    x: 4,
    y: 0,
  },
  {
    h: 4,
    id: "add-widget::inline-add",
    minH: 1,
    minW: 1,
    w: 4,
    widgetType: "add-widget",
    x: 8,
    y: 0,
  },
];

const successfulWrite = {
  diagnostics: [],
  instanceHighWaterMarks: { "work-outcome-chart": 1 },
  persisted: true,
};

function createOptions(
  overrides: Partial<Parameters<typeof useDashboardWidgetRemoval>[0]> = {},
) {
  return {
    dashboardLayout: layout,
    dirtyCardInstanceIDs: new Set<string>(),
    getWidgetTitle: (widgetType: string) => widgetType,
    persistDashboardLayout: vi.fn(() => successfulWrite),
    removeDashboardWidget: vi.fn(() => successfulWrite),
    ...overrides,
  };
}

function appendRemoveButton(widgetInstanceID: string) {
  const button = document.createElement("button");
  button.dataset.dashboardWidgetRemove = "true";
  button.dataset.dashboardWidgetInstanceId = widgetInstanceID;
  document.body.append(button);
  return button;
}

function appendAddButton() {
  const button = document.createElement("button");
  button.dataset.dashboardAddWidgetControl = "true";
  document.body.append(button);
  return button;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: removal scenarios share one fake-timer and DOM cleanup harness.
describe("useDashboardWidgetRemoval", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    cleanup();
    document.body.innerHTML = "";
  });

  describe("clean removal", () => {
    it("removes clean cards, exposes bounded undo, and restores the exact instance geometry", async () => {
      const removeButton = appendRemoveButton("work-totals::primary");
      const nearestButton = appendRemoveButton(
        "work-outcome-chart::instance-1",
      );
      const addButton = appendAddButton();
      const options = createOptions();
      const { result, rerender } = renderHook(
        ({ dashboardLayout }) =>
          useDashboardWidgetRemoval({ ...options, dashboardLayout }),
        { initialProps: { dashboardLayout: layout } },
      );

      removeButton.focus();
      act(() => {
        result.current.requestRemoval("work-totals::primary");
      });

      expect(options.removeDashboardWidget).toHaveBeenCalledWith(
        "work-totals::primary",
      );
      expect(result.current.undoState).toMatchObject({
        status: "available",
        widgetTitle: "work-totals",
      });

      rerender({
        dashboardLayout: layout.filter(
          (item) => item.id !== "work-totals::primary",
        ),
      });
      act(() => {
        vi.advanceTimersByTime(0);
      });
      expect(document.activeElement).toBe(nearestButton);

      act(() => {
        result.current.undoRemoval();
      });

      expect(options.persistDashboardLayout).toHaveBeenCalledWith(
        expect.arrayContaining([
          expect.objectContaining({
            h: 2,
            id: "work-totals::primary",
            x: 0,
            y: 0,
          }),
          expect.objectContaining({ id: "work-outcome-chart::instance-1" }),
        ]),
      );
      expect(result.current.undoState?.status).toBe("restored");

      rerender({ dashboardLayout: layout });
      act(() => {
        vi.runOnlyPendingTimers();
      });
      expect(document.activeElement).toBe(removeButton);
      expect(addButton).not.toBe(document.activeElement);
    });
  });

  describe("dirty removal", () => {
    it("confirms dirty removal and keeps cancel focus and persistence unchanged", async () => {
      const removeButton = appendRemoveButton("work-outcome-chart::instance-1");
      const options = createOptions({
        dirtyCardInstanceIDs: new Set(["work-outcome-chart::instance-1"]),
      });
      const { result } = renderHook(() => useDashboardWidgetRemoval(options));

      removeButton.focus();
      act(() => {
        result.current.requestRemoval("work-outcome-chart::instance-1");
      });

      expect(result.current.pendingRemoval?.widgetTitle).toBe(
        "work-outcome-chart",
      );
      expect(options.removeDashboardWidget).not.toHaveBeenCalled();

      act(() => {
        result.current.cancelRemoval();
      });
      expect(result.current.pendingRemoval).toBeNull();
      act(() => {
        vi.runOnlyPendingTimers();
      });
      expect(document.activeElement).toBe(removeButton);

      act(() => {
        result.current.requestRemoval("work-outcome-chart::instance-1");
      });
      act(() => {
        result.current.confirmRemoval();
      });
      expect(options.removeDashboardWidget).toHaveBeenCalledWith(
        "work-outcome-chart::instance-1",
      );
    });
  });

  describe("persistence failures", () => {
    it("marks undo as failed while keeping the in-memory restoration usable", () => {
      const options = createOptions({
        persistDashboardLayout: vi.fn(() => ({
          ...successfulWrite,
          persisted: false,
        })),
      });
      const { result, rerender } = renderHook(
        ({ dashboardLayout }) =>
          useDashboardWidgetRemoval({ ...options, dashboardLayout }),
        { initialProps: { dashboardLayout: layout } },
      );

      act(() => {
        result.current.requestRemoval("work-totals::primary");
      });
      rerender({
        dashboardLayout: layout.filter(
          (item) => item.id !== "work-totals::primary",
        ),
      });
      act(() => {
        result.current.undoRemoval();
      });

      expect(result.current.undoState?.status).toBe("failed-to-persist");
      expect(options.persistDashboardLayout).toHaveBeenCalled();
    });
  });

  describe("expiry", () => {
    it("exposes an expired undo state and falls back to the add control", () => {
      const addButton = appendAddButton();
      const options = createOptions();
      const { result, rerender } = renderHook(
        ({ dashboardLayout }) =>
          useDashboardWidgetRemoval({ ...options, dashboardLayout }),
        { initialProps: { dashboardLayout: layout } },
      );

      act(() => {
        result.current.requestRemoval("work-totals::primary");
      });
      rerender({
        dashboardLayout: layout.filter(
          (item) => item.id !== "work-totals::primary",
        ),
      });
      act(() => {
        vi.advanceTimersByTime(DASHBOARD_WIDGET_UNDO_TIMEOUT_MS);
      });

      expect(result.current.undoState?.status).toBe("expired");
      act(() => {
        vi.runOnlyPendingTimers();
      });
      expect(document.activeElement).toBe(addButton);
    });
  });
});

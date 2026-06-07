import { expect } from "vitest";

import type { DashboardTrace } from "../../../api/dashboard/types";

export function buildLongDispatchTrace(
  dispatchCount: number,
  idPrefix = "dispatch-scroll",
): DashboardTrace {
  return {
    dispatches: Array.from({ length: dispatchCount }, (_, index) => ({
      current_chaining_trace_id: `trace-chain-${index}`,
      dispatch_id: `${idPrefix}-${index}`,
      duration_millis: 1000 + index,
      end_time: `2026-04-08T12:00:${String(index + 1).padStart(2, "0")}Z`,
      outcome: "ACCEPTED" as const,
      start_time: `2026-04-08T12:00:${String(index).padStart(2, "0")}Z`,
      transition_id: `transition-${index}`,
      workstation_name: `Workstation ${index}`,
    })),
    trace_id: "trace-scroll-card",
    transition_ids: [],
    work_ids: [],
    workstation_sequence: Array.from(
      { length: dispatchCount },
      (_, index) => `Workstation ${index}`,
    ),
  };
}

export function expectNoVerticalScrollContainer(
  element: Element,
  options?: { requireOverflowYClip?: boolean },
): void {
  if (options?.requireOverflowYClip) {
    expect(element.className).toContain("overflow-y-clip");
  }
  expect(element.className).not.toMatch(/overflow-y-(auto|scroll)/);
  const style = window.getComputedStyle(element);
  expect(style.overflowY).not.toBe("auto");
  expect(style.overflowY).not.toBe("scroll");
}

export function expectPageFlowCardBody(element: HTMLElement): void {
  expect(element.hasAttribute("data-radix-scroll-area-viewport")).toBe(true);
}

export function findTraceCardBody(card: HTMLElement): HTMLElement {
  const cardBody = card.querySelector("[data-trace-card-body]");
  if (!(cardBody instanceof HTMLElement)) {
    throw new Error("Expected trace card body.");
  }

  expectPageFlowCardBody(cardBody);
  return cardBody;
}

export function findTraceDispatchTableRegion(card: HTMLElement): HTMLElement {
  const tableRegion = card.querySelector("[data-trace-dispatch-table]");
  if (!(tableRegion instanceof HTMLElement)) {
    throw new Error("Expected trace dispatch table region to render.");
  }

  return tableRegion;
}

export function expectNoVerticalScrollBetweenDispatchTableAndCardBody(
  card: HTMLElement,
): void {
  const cardBody = findTraceCardBody(card);
  const tableRegion = findTraceDispatchTableRegion(card);

  let current: Element | null = tableRegion;
  while (current && current !== cardBody) {
    expectNoVerticalScrollContainer(current);
    current = current.parentElement;
  }

  expect(current).toBe(cardBody);
  expectPageFlowCardBody(cardBody);
}

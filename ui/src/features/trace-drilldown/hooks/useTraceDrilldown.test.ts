import { describe, expect, it } from "vitest";

import type { DashboardTrace } from "../../../api/dashboard/types";
import { resolveTraceGridState } from "./useTraceDrilldown";

function buildTrace(overrides: Partial<DashboardTrace> = {}): DashboardTrace {
  return {
    dispatches: [],
    relations: [],
    trace_id: "trace-selected",
    transition_ids: [],
    work_ids: ["work-selected"],
    workstation_sequence: [],
    ...overrides,
  };
}

function resolve(
  overrides: Partial<Parameters<typeof resolveTraceGridState>[0]> = {},
) {
  return resolveTraceGridState({
    error: null,
    idleMessage: "Select work",
    isLoading: false,
    selectedTrace: buildTrace(),
    selectedWorkID: "work-selected",
    ...overrides,
  });
}

describe("resolveTraceGridState", () => {
  it("gives query errors precedence over known-empty classification", () => {
    expect(resolve({ error: new Error("trace read failed") })).toEqual({
      message: "trace read failed",
      status: "error",
    });
  });

  it("treats relation-only traces as successful content", () => {
    const trace = buildTrace({
      relations: [
        {
          source_work_id: "work-source",
          target_work_id: "work-selected",
          type: "DEPENDS_ON",
        },
      ],
    });

    expect(resolve({ selectedTrace: trace })).toEqual({
      status: "ready",
      trace,
    });
  });

  it("keeps loading, known-empty, and dispatch-bearing success distinct", () => {
    expect(resolve({ isLoading: true })).toEqual({
      status: "loading",
      workID: "work-selected",
    });
    expect(resolve()).toEqual({ status: "empty", workID: "work-selected" });

    const trace = buildTrace({
      dispatches: [
        {
          dispatch_id: "dispatch-selected",
          end_time: "2026-08-15T12:00:01Z",
          outcome: "ACCEPTED",
          start_time: "2026-08-15T12:00:00Z",
          transition_id: "process",
        },
      ],
    });
    expect(resolve({ selectedTrace: trace })).toEqual({
      status: "ready",
      trace,
    });
  });

  it("keeps the unselected state explicit", () => {
    expect(resolve({ selectedWorkID: null })).toEqual({
      message: "Select work",
      status: "idle",
    });
  });
});

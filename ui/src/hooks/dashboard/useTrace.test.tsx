import { expandTraceWithCausalPredecessors } from "./useTrace";
import type { DashboardTrace } from "../../api/dashboard/types";
import { describe, expect, it } from "vitest";

function buildTrace(
  traceID: string,
  overrides: Partial<DashboardTrace> = {},
): DashboardTrace {
  return {
    dispatches: [],
    relations: [],
    request_ids: [],
    trace_id: traceID,
    transition_ids: [],
    work_ids: [],
    work_items: [],
    workstation_sequence: [],
    ...overrides,
  };
}

describe("expandTraceWithCausalPredecessors", () => {
  it("returns the original trace when no indexed predecessors exist", () => {
    const trace = buildTrace("trace-current", {
      dispatches: [
        {
          dispatch_id: "dispatch-current",
          end_time: "2026-04-22T18:00:02.000Z",
          outcome: "ACCEPTED",
          previous_chaining_trace_ids: ["trace-missing"],
          start_time: "2026-04-22T18:00:01.000Z",
          transition_id: "review",
        },
      ],
      request_ids: ["request-current"],
      transition_ids: ["review"],
      work_ids: ["work-current"],
      workstation_sequence: ["Review"],
    });

    expect(expandTraceWithCausalPredecessors(trace, {})).toBe(trace);
  });

  it("merges predecessor traces into a single ordered trace view", () => {
    const predecessorTrace = buildTrace("trace-b", {
      dispatches: [
        {
          dispatch_id: "dispatch-b",
          end_time: "2026-04-22T18:00:02.000Z",
          outcome: "ACCEPTED",
          start_time: "2026-04-22T18:00:01.000Z",
          transition_id: "research",
          workstation_name: "Research",
        },
      ],
      request_ids: ["request-b"],
      relations: [
        {
          target_work_id: "work-b",
          type: "blocks",
        },
      ],
      transition_ids: ["research"],
      work_ids: ["work-b"],
      workstation_sequence: ["Research"],
    });
    const currentTrace = buildTrace("trace-a", {
      dispatches: [
        {
          dispatch_id: "dispatch-a",
          end_time: "2026-04-22T18:00:04.000Z",
          input_items: [
            {
              display_name: "Research context",
              previous_chaining_trace_ids: ["trace-b"],
              work_id: "work-b",
              work_type_id: "story",
            },
          ],
          outcome: "ACCEPTED",
          start_time: "2026-04-22T18:00:03.000Z",
          transition_id: "implement",
          workstation_name: "Implement",
        },
      ],
      request_ids: ["request-a"],
      transition_ids: ["implement"],
      work_ids: ["work-a"],
      workstation_sequence: ["Implement"],
    });

    const expanded = expandTraceWithCausalPredecessors(currentTrace, {
      "work-a": currentTrace,
      "work-b": predecessorTrace,
    });

    expect(expanded).toMatchObject({
      request_ids: ["request-a", "request-b"],
      transition_ids: ["implement", "research"],
      work_ids: ["work-a", "work-b"],
      workstation_sequence: ["Research", "Implement"],
    });
    expect(expanded?.dispatches.map((dispatch) => dispatch.dispatch_id)).toEqual([
      "dispatch-b",
      "dispatch-a",
    ]);
    expect(expanded?.work_items).toBeUndefined();
  });

  it("deduplicates repeated predecessor dispatches and relations", () => {
    const sharedRelation = {
      request_id: "request-shared",
      source_work_id: "work-b",
      target_work_id: "work-a",
      type: "blocks" as const,
    };
    const predecessorTrace = buildTrace("trace-b", {
      dispatches: [
        {
          dispatch_id: "dispatch-b",
          end_time: "2026-04-22T18:00:02.000Z",
          outcome: "ACCEPTED",
          start_time: "2026-04-22T18:00:01.000Z",
          transition_id: "review",
        },
      ],
      relations: [sharedRelation],
      work_ids: ["work-b"],
    });
    const currentTrace = buildTrace("trace-a", {
      dispatches: [
        {
          dispatch_id: "dispatch-b",
          end_time: "2026-04-22T18:00:02.000Z",
          outcome: "ACCEPTED",
          previous_chaining_trace_ids: ["trace-b"],
          start_time: "2026-04-22T18:00:01.000Z",
          transition_id: "review",
        },
      ],
      relations: [sharedRelation],
      work_ids: ["work-a"],
    });

    const expanded = expandTraceWithCausalPredecessors(currentTrace, {
      "work-a": currentTrace,
      "work-b": predecessorTrace,
    });

    expect(expanded?.dispatches).toHaveLength(1);
    expect(expanded?.relations).toEqual([sharedRelation]);
  });
});

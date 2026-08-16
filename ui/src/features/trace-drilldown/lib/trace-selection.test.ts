import { describe, expect, it } from "vitest";
import {
  traceSelectionIdentitiesByWorkID,
  traceSelectionIdentitiesForDispatch,
  traceSelectionKey,
  traceSelectionMatches,
} from "./trace-selection";

describe("trace selection identity", () => {
  it("requires dispatch, work, and attempt to match", () => {
    const selection = {
      attempt: 2,
      dispatch_id: "dispatch-retry",
      work_id: "work-shared",
    };

    expect(traceSelectionMatches(selection, selection)).toBe(true);
    expect(
      traceSelectionMatches(selection, {
        ...selection,
        attempt: 1,
      }),
    ).toBe(false);
    expect(
      traceSelectionMatches(selection, {
        ...selection,
        dispatch_id: "dispatch-other",
      }),
    ).toBe(false);
    expect(
      traceSelectionMatches(selection, {
        ...selection,
        work_id: "work-other",
      }),
    ).toBe(false);
    expect(traceSelectionMatches(selection, null)).toBe(false);
  });

  it("projects each dispatch work item into a stable tuple key", () => {
    const identities = traceSelectionIdentitiesForDispatch({
      attempt: 2,
      dispatch_id: "dispatch-retry",
      input_items: [{ work_id: "work-shared" }],
      output_items: [{ work_id: "work-result" }],
    });

    expect(identities).toEqual([
      {
        attempt: 2,
        dispatch_id: "dispatch-retry",
        work_id: "work-shared",
      },
      {
        attempt: 2,
        dispatch_id: "dispatch-retry",
        work_id: "work-result",
      },
    ]);
    expect(new Set(identities.map(traceSelectionKey)).size).toBe(2);
  });

  it("indexes every Work identity without collapsing attempts", () => {
    const selectionsByWorkID = traceSelectionIdentitiesByWorkID([
      {
        attempt: 1,
        dispatch_id: "dispatch-retry",
        input_items: [{ work_id: "work-shared" }],
      },
      {
        attempt: 2,
        dispatch_id: "dispatch-retry",
        input_items: [{ work_id: "work-shared" }],
      },
    ]);

    expect(selectionsByWorkID.get("work-shared")).toEqual([
      {
        attempt: 1,
        dispatch_id: "dispatch-retry",
        work_id: "work-shared",
      },
      {
        attempt: 2,
        dispatch_id: "dispatch-retry",
        work_id: "work-shared",
      },
    ]);
  });
});

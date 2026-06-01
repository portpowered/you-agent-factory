import { describe, expect, it } from "vitest";

import {
  createDefaultInputGuard,
  createDefaultWorkstationGuard,
  editableWorkstationDraftsEqual,
  formatWorkstationGuardSummary,
  guardsDraftEqual,
  normalizeEditableInputGuards,
  setEditableInputSlotGuard,
} from "./workstation-guards";

describe("workstation-guards", () => {
  it("formats VISIT_COUNT and MATCHES_FIELDS summaries", () => {
    expect(
      formatWorkstationGuardSummary({
        maxVisits: 2,
        type: "VISIT_COUNT",
        workstation: "Review",
      }),
    ).toBe("Review · max 2");
    expect(
      formatWorkstationGuardSummary({
        matchConfig: { inputKey: '.Tags["flavor"]' },
        type: "MATCHES_FIELDS",
      }),
    ).toBe('.Tags["flavor"]');
  });

  it("creates default guards with factory workstation names", () => {
    expect(
      createDefaultWorkstationGuard("VISIT_COUNT", ["Plan", "Review"]),
    ).toEqual({
      maxVisits: 1,
      type: "VISIT_COUNT",
      workstation: "Plan",
    });
    expect(createDefaultWorkstationGuard("MATCHES_FIELDS", [])).toEqual({
      matchConfig: { inputKey: ".Name" },
      type: "MATCHES_FIELDS",
    });
  });

  it("normalizes input guards to at most one entry", () => {
    expect(
      normalizeEditableInputGuards([
        { matchInput: "planItem", type: "SAME_NAME" },
        { parentInput: "planItem", type: "ALL_CHILDREN_COMPLETE" },
      ]),
    ).toEqual([{ matchInput: "planItem", type: "SAME_NAME" }]);
    expect(normalizeEditableInputGuards([])).toEqual([]);
  });

  it("creates default per-input guards and clears slot guards", () => {
    expect(createDefaultInputGuard("SAME_NAME", ["planItem", "story"])).toEqual(
      {
        matchInput: "planItem",
        type: "SAME_NAME",
      },
    );
    expect(
      createDefaultInputGuard("ALL_CHILDREN_COMPLETE", ["planItem"]),
    ).toEqual({
      parentInput: "planItem",
      type: "ALL_CHILDREN_COMPLETE",
    });

    const input = {
      guards: [{ matchInput: "planItem", type: "SAME_TRACE_ID" as const }],
      state: "queued",
      workType: "story",
    };
    expect(setEditableInputSlotGuard(input, null)).toEqual({
      guards: [],
      state: "queued",
      workType: "story",
    });
  });

  it("compares guard and full draft equality", () => {
    const left = {
      behavior: "STANDARD" as const,
      guards: [
        { maxVisits: 1, type: "VISIT_COUNT" as const, workstation: "A" },
      ],
      inputs: [],
      prompt: "",
      runnerName: null,
      workerName: "w",
    };
    const right = {
      ...left,
      guards: [
        { maxVisits: 1, type: "VISIT_COUNT" as const, workstation: "A" },
      ],
    };
    const changed = {
      ...left,
      guards: [
        { maxVisits: 2, type: "VISIT_COUNT" as const, workstation: "A" },
      ],
    };

    expect(guardsDraftEqual(left.guards, right.guards)).toBe(true);
    expect(editableWorkstationDraftsEqual(left, right)).toBe(true);
    expect(editableWorkstationDraftsEqual(left, changed)).toBe(false);
  });
});

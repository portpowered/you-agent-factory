import { describe, expect, it } from "bun:test";

import {
  createDefaultInputGuard,
  createDefaultWorkstationGuard,
  editableWorkstationDraftsEqual,
  formatInputGuardSummary,
  formatWorkstationGuardSummary,
  guardsDraftEqual,
  normalizeEditableInputGuards,
  resolvePeerInputWorkTypes,
  rewriteVisitCountWorkstationReference,
  rewriteWorkstationVisitCountReferences,
  setEditableInputSlotGuard,
} from "./workstation-guards";

describe("workstation-guards formatting and defaults", () => {
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

  it("formats input guard summaries and resolves peer work types", () => {
    expect(
      formatInputGuardSummary({
        matchInput: "planItem",
        type: "SAME_NAME",
      }),
    ).toBe("planItem");
    expect(
      formatInputGuardSummary({
        parentInput: "story",
        spawnedBy: "split-story",
        type: "ALL_CHILDREN_COMPLETE",
      }),
    ).toBe("story · split-story");
    expect(
      resolvePeerInputWorkTypes(
        [
          { guards: [], state: "queued", workType: "story" },
          { guards: [], state: "complete", workType: "task" },
        ],
        0,
      ),
    ).toEqual(["task"]);
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
});

describe("workstation-guards draft equality and rename", () => {
  it("compares guard and full draft equality", () => {
    const left = {
      behavior: "STANDARD" as const,
      cron: null,
      guards: [
        { maxVisits: 1, type: "VISIT_COUNT" as const, workstation: "A" },
      ],
      inputs: [],
      name: "Alpha",
      operation: "",
      operationBindings: [],
      prompt: "",
      runnerName: null,
      workerName: "w",
      workstationType: "MODEL_WORKSTATION" as const,
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
    expect(
      editableWorkstationDraftsEqual(left, { ...left, name: "  Alpha  " }),
    ).toBe(true);
    expect(
      editableWorkstationDraftsEqual(left, { ...left, name: "Beta" }),
    ).toBe(false);
  });

  it("rewrites VISIT_COUNT workstation references on workstation and input guards", () => {
    expect(
      rewriteVisitCountWorkstationReference(
        { maxVisits: 2, type: "VISIT_COUNT", workstation: "Plan" },
        "Plan",
        "Planning",
      ),
    ).toEqual({
      maxVisits: 2,
      type: "VISIT_COUNT",
      workstation: "Planning",
    });
    expect(
      rewriteVisitCountWorkstationReference(
        { maxVisits: 2, type: "VISIT_COUNT", workstation: "Review" },
        "Plan",
        "Planning",
      ),
    ).toEqual({
      maxVisits: 2,
      type: "VISIT_COUNT",
      workstation: "Review",
    });

    expect(
      rewriteWorkstationVisitCountReferences(
        {
          guards: [{ maxVisits: 1, type: "VISIT_COUNT", workstation: "Plan" }],
          inputs: [
            {
              guards: [
                { maxVisits: 3, type: "VISIT_COUNT", workstation: "Plan" },
              ],
            },
          ],
        },
        "Plan",
        "Planning",
      ),
    ).toEqual({
      guards: [{ maxVisits: 1, type: "VISIT_COUNT", workstation: "Planning" }],
      inputs: [
        {
          guards: [
            { maxVisits: 3, type: "VISIT_COUNT", workstation: "Planning" },
          ],
        },
      ],
    });
  });
});

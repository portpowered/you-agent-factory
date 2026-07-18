import type { FactoryDefinition } from "@you-agent-factory/client";
import { describe, expect, it } from "vitest";

import exampleScenario from "../examples/customer-support.scenario.v1.json" with {
  type: "json",
};
import {
  FACTORY_EMULATOR_SCENARIO_SCHEMA_VERSION,
  FactoryEmulatorScenarioValidationError,
  parseFactoryEmulatorScenario,
  safeParseFactoryEmulatorScenario,
  scenarioSchema,
} from "./scenario.js";

const factory = {
  name: "customer-support",
  workers: [{ name: "support-agent" }],
  workTypes: [
    {
      name: "ticket",
      states: [
        { name: "new", type: "INITIAL" },
        { name: "classified", type: "PROCESSING" },
      ],
    },
  ],
  workstations: [
    {
      name: "triage",
      worker: "support-agent",
      inputs: [{ workType: "ticket", state: "new" }],
    },
  ],
} satisfies FactoryDefinition;

function scenarioWith(changes: Record<string, unknown> = {}): unknown {
  return { ...structuredClone(exampleScenario), ...changes };
}

function expectIssue(input: unknown, code: string, path?: readonly unknown[]) {
  const result = safeParseFactoryEmulatorScenario(input, factory);
  expect(result.success).toBe(false);
  if (result.success) return;
  expect(result.issues).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        code,
        ...(path === undefined ? {} : { path }),
      }),
    ]),
  );
}

describe("Factory emulator scenario contract", () => {
  it("publishes and parses the checked-in versioned example", () => {
    expect(scenarioSchema.$id).toContain("/ui/factory-emulator/scenario/v1");
    expect(scenarioSchema.properties.schemaVersion.const).toBe(
      FACTORY_EMULATOR_SCENARIO_SCHEMA_VERSION,
    );

    const parsed = parseFactoryEmulatorScenario(exampleScenario, factory);
    expect(parsed).toEqual(exampleScenario);
    expect(parsed).not.toBe(exampleScenario);
    expect(parsed.rules).not.toBe(exampleScenario.rules);
  });

  it("preserves authored rule order and treats omitted selector fields as wildcards", () => {
    const rules = [
      {
        ...structuredClone(exampleScenario.rules[0]),
        id: "specific-state",
        selector: { input: { workType: "ticket", state: "new" } },
      },
      {
        ...structuredClone(exampleScenario.rules[0]),
        id: "different-state",
        selector: { input: { workType: "ticket", state: "classified" } },
      },
    ];
    const result = safeParseFactoryEmulatorScenario(
      scenarioWith({ rules }),
      factory,
    );
    expect(result).toMatchObject({
      success: true,
      data: { rules: [{ id: "specific-state" }, { id: "different-state" }] },
    });
  });

  it("reports unsupported versions, unstable identities, and non-normalized UTC", () => {
    expectIssue(
      scenarioWith({ schemaVersion: "factory-emulator-scenario/v2" }),
      "unsupported_schema_version",
      ["schemaVersion"],
    );
    expectIssue(scenarioWith({ id: "not stable" }), "unstable_identity", [
      "id",
    ]);
    expectIssue(
      scenarioWith({ startAt: "2026-07-18T09:00:00-07:00" }),
      "invalid_start_at",
      ["startAt"],
    );
    expectIssue(
      scenarioWith({ startAt: "2026-07-18T16:00:00Z" }),
      "invalid_start_at",
      ["startAt"],
    );
  });

  it("reports duplicate rule and initial-submission identities", () => {
    const duplicateRule = structuredClone(exampleScenario.rules[0]);
    const duplicateSubmission = structuredClone(
      exampleScenario.initialSubmissions[0],
    );
    const input = scenarioWith({
      rules: [structuredClone(duplicateRule), structuredClone(duplicateRule)],
      initialSubmissions: [
        structuredClone(duplicateSubmission),
        structuredClone(duplicateSubmission),
      ],
    });
    const result = safeParseFactoryEmulatorScenario(input, factory);
    expect(result).toMatchObject({ success: false });
    if (result.success) return;
    expect(
      result.issues.filter((issue) => issue.code === "duplicate_identity"),
    ).toHaveLength(4);
  });

  it("detects fully shadowed first-match rules including wildcard selectors", () => {
    const wildcard = {
      ...structuredClone(exampleScenario.rules[0]),
      id: "all-work",
      selector: {},
    };
    const specific = {
      ...structuredClone(exampleScenario.rules[0]),
      id: "only-triage",
    };
    expectIssue(
      scenarioWith({ rules: [wildcard, specific] }),
      "fully_shadowed_rule",
      ["rules", 1, "selector"],
    );
  });
});

describe("Factory emulator scenario semantics", () => {
  it("validates Factory identity and every selector reference", () => {
    expectIssue(
      scenarioWith({ factory: { name: "another-factory" } }),
      "invalid_factory_identity",
      ["factory", "name"],
    );
    for (const selector of [
      { workstation: "missing" },
      { worker: "missing" },
      { input: { workType: "missing" } },
      { input: { state: "new" } },
      { input: { workType: "ticket", state: "missing" } },
    ]) {
      const rule = { ...structuredClone(exampleScenario.rules[0]), selector };
      expectIssue(
        scenarioWith({ rules: [rule] }),
        "invalid_selector_reference",
      );
    }
  });

  it("requires a lineage cursor and explicit repeat-last or fail exhaustion", () => {
    const withoutCursor = structuredClone(exampleScenario.rules[0]) as Record<
      string,
      unknown
    >;
    delete withoutCursor.cursor;
    expectIssue(
      scenarioWith({ rules: [withoutCursor] }),
      "missing_required_field",
      ["rules", 0, "cursor"],
    );

    const invalidCursor = {
      ...structuredClone(exampleScenario.rules[0]),
      cursor: { scope: "global", input: "workId" },
    };
    expectIssue(scenarioWith({ rules: [invalidCursor] }), "invalid_cursor");

    const invalidExhaustion = {
      ...structuredClone(exampleScenario.rules[0]),
      exhaustion: "cycle",
    };
    expectIssue(
      scenarioWith({ rules: [invalidExhaustion] }),
      "invalid_exhaustion",
      ["rules", 0, "exhaustion"],
    );
  });

  it("requires non-empty ordered outcomes with valid finite durations", () => {
    const noOutcomes = {
      ...structuredClone(exampleScenario.rules[0]),
      outcomes: [],
    };
    expectIssue(scenarioWith({ rules: [noOutcomes] }), "invalid_outcome", [
      "rules",
      0,
      "outcomes",
    ]);

    for (const durationMs of [-1, Number.NaN, Number.POSITIVE_INFINITY]) {
      const rule = {
        ...structuredClone(exampleScenario.rules[0]),
        outcomes: [{ result: "accepted", durationMs }],
      };
      expectIssue(scenarioWith({ rules: [rule] }), "invalid_outcome");
    }
  });

  it("enforces result-specific error fields and bounded plain-text metadata", () => {
    const invalidOutcomes = [
      { result: "failed", durationMs: 0 },
      { result: "accepted", durationMs: 0, error: "not allowed" },
      { result: "continued", durationMs: 0, activityLabel: "x".repeat(121) },
      { result: "rejected", durationMs: 0, feedback: "x".repeat(4097) },
      { result: "accepted", durationMs: 0, output: "x".repeat(65_537) },
      { result: "unknown", durationMs: 0 },
    ];
    for (const outcome of invalidOutcomes) {
      const rule = {
        ...structuredClone(exampleScenario.rules[0]),
        outcomes: [outcome],
      };
      expectIssue(scenarioWith({ rules: [rule] }), "invalid_outcome");
    }
  });
});

describe("Factory emulator scenario boundaries", () => {
  it("accepts only error or one concrete valid unmatched outcome", () => {
    expect(
      safeParseFactoryEmulatorScenario(
        scenarioWith({ unmatched: { behavior: "error" } }),
        factory,
      ),
    ).toMatchObject({ success: true });
    expect(
      safeParseFactoryEmulatorScenario(
        scenarioWith({
          unmatched: {
            behavior: "outcome",
            outcome: {
              result: "rejected",
              durationMs: 0,
              feedback: "No scripted rule",
            },
          },
        }),
        factory,
      ),
    ).toMatchObject({ success: true });
    expectIssue(
      scenarioWith({ unmatched: { behavior: "passthrough" } }),
      "invalid_unmatched",
    );
  });

  it("rejects prohibited executable fields at the contract boundary", () => {
    for (const field of [
      "passthroughWorker",
      "providerDeltas",
      "retries",
      "costs",
      "script",
      "spawnedWork",
    ]) {
      expectIssue(scenarioWith({ [field]: true }), "unsupported_field", [
        field,
      ]);
    }
  });

  it("accepts an empty initial batch and atomically rejects invalid submissions", () => {
    expect(
      safeParseFactoryEmulatorScenario(
        scenarioWith({ initialSubmissions: [] }),
        factory,
      ),
    ).toMatchObject({ success: true });

    const invalidBatch = [
      structuredClone(exampleScenario.initialSubmissions[0]),
      { name: "child", workType: "ticket", state: "new", parent: "missing" },
    ];
    const result = safeParseFactoryEmulatorScenario(
      scenarioWith({ initialSubmissions: invalidBatch }),
      factory,
    );
    expect(result).toMatchObject({ success: false });
    expect(result).not.toHaveProperty("data");
    if (result.success) return;
    expect(result.issues).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          code: "invalid_initial_submission_relationship",
          path: ["initialSubmissions", 1, "parent"],
        }),
      ]),
    );
  });

  it("rejects cyclic submission relationships and invalid Work references", () => {
    expectIssue(
      scenarioWith({
        initialSubmissions: [
          { name: "first", workType: "ticket", state: "new", parent: "second" },
          { name: "second", workType: "ticket", state: "new", parent: "first" },
        ],
      }),
      "invalid_initial_submission_relationship",
    );
    expectIssue(
      scenarioWith({
        initialSubmissions: [
          { name: "bad", workType: "missing", state: "new" },
        ],
      }),
      "invalid_selector_reference",
      ["initialSubmissions", 0, "workType"],
    );
  });

  it("throws the structured validation error from the strict parser", () => {
    expect(() => parseFactoryEmulatorScenario({}, factory)).toThrow(
      FactoryEmulatorScenarioValidationError,
    );
  });
});

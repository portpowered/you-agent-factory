import assert from "node:assert/strict";
import test from "node:test";
import {
  parseEmulatorScenario,
  resolveEmulatorScenarioResult,
  selectEmulatorRule,
  SUPPORTED_SCENARIO_VERSION,
} from "@you-agent-factory/factory-emulator";

function supportedFactory(overrides = {}) {
  return {
    name: "checkout",
    workTypes: [{ name: "checkout", states: [] }],
    workstations: [{ name: "complete", worker: "emulator", inputs: [] }],
    ...overrides,
  };
}

function validScenario(overrides = {}) {
  return {
    version: SUPPORTED_SCENARIO_VERSION,
    id: "deterministic-checkout",
    seed: "seed-0001",
    startAt: "2026-07-18T07:30:00Z",
    rules: [
      {
        id: "complete-checkout",
        match: { kind: "workType", workType: "checkout" },
        outcomes: [{ kind: "complete" }],
        exhaustionBehavior: { kind: "repeatLast" },
      },
    ],
    unmatchedBehavior: { kind: "ignore" },
    ...overrides,
  };
}

test("parses a valid scenario without changing deterministic inputs or emitting activity", () => {
  const scenario = validScenario({ activityLabel: "Running checkout" });
  const factory = supportedFactory();

  const result = parseEmulatorScenario(scenario, factory);

  assert.deepEqual(result, { success: true, scenario, factory });
});

test("reports actionable structural diagnostics before Factory support checks", () => {
  const result = parseEmulatorScenario(
    validScenario({ id: "", unexpected: true }),
    supportedFactory({ orchestrator: { kind: "JAVASCRIPT" } }),
  );

  assert.equal(result.success, false);
  assert.deepEqual(
    result.diagnostics.map(({ code, path, expectation }) => ({
      code,
      path,
      expectation,
    })),
    [
      {
        code: "INVALID_SCENARIO_SHAPE",
        path: "/id",
        expectation: "must NOT have fewer than 1 characters",
      },
      {
        code: "INVALID_SCENARIO_SHAPE",
        path: "/unexpected",
        expectation: 'no additional property "unexpected"',
      },
    ],
  );
});

test("rejects unsupported executable Factory definitions before emulator activity", () => {
  const result = parseEmulatorScenario(
    validScenario(),
    supportedFactory({
      orchestrator: { kind: "JAVASCRIPT" },
      resources: [{ name: "provider-quota", capacity: 1 }],
      guards: [{ type: "VISIT_COUNT" }],
      workstations: [
        { name: "clock", behavior: "CRON", cron: { schedule: "* * * * *" } },
      ],
    }),
  );

  assert.equal(result.success, false);
  assert.deepEqual(
    result.diagnostics.map(({ code, path }) => ({ code, path })),
    [
      { code: "UNSUPPORTED_FACTORY_CAPABILITY", path: "/orchestrator/kind" },
      { code: "UNSUPPORTED_FACTORY_CAPABILITY", path: "/resources" },
      { code: "UNSUPPORTED_FACTORY_CAPABILITY", path: "/guards" },
      { code: "UNSUPPORTED_FACTORY_CAPABILITY", path: "/workstations/0/behavior" },
      { code: "UNSUPPORTED_FACTORY_CAPABILITY", path: "/workstations/0/cron" },
    ],
  );
});

test("accepts activityLabel only through the bounded scenario contract", () => {
  const result = parseEmulatorScenario(
    validScenario({ activityLabel: "x".repeat(121) }),
    supportedFactory(),
  );

  assert.equal(result.success, false);
  assert.equal(result.diagnostics[0].path, "/activityLabel");
});

test("validates initial submissions and backward complete lineage cursors", () => {
  const scenario = validScenario({
    initialSubmissions: [{ id: "checkout-1", workType: "checkout" }],
    rules: [
      {
        id: "complete-checkout",
        match: { kind: "workType", workType: "checkout" },
        outcomes: [
          {
            kind: "complete",
            lineageCursor: {
              kind: "initialSubmission",
              submissionId: "checkout-1",
            },
          },
        ],
        exhaustionBehavior: { kind: "repeatLast" },
      },
      {
        id: "complete-fulfillment",
        match: { kind: "workType", workType: "fulfillment" },
        outcomes: [
          {
            kind: "complete",
            lineageCursor: {
              kind: "scriptedOutcome",
              ruleId: "complete-checkout",
              outcomeIndex: 0,
            },
          },
        ],
        exhaustionBehavior: { kind: "useUnmatchedBehavior" },
      },
    ],
  });

  const result = parseEmulatorScenario(
    scenario,
    supportedFactory({
      workTypes: [{ name: "checkout" }, { name: "fulfillment" }],
    }),
  );

  assert.deepEqual(result, {
    success: true,
    scenario,
    factory: supportedFactory({
      workTypes: [{ name: "checkout" }, { name: "fulfillment" }],
    }),
  });
});

for (const invalidCase of [
  {
    name: "an unknown Factory work type",
    scenario: validScenario({
      initialSubmissions: [{ id: "unknown", workType: "unknown" }],
    }),
    code: "UNKNOWN_FACTORY_WORK_TYPE",
  },
  {
    name: "a missing initial-submission lineage target",
    scenario: validScenario({
      rules: [
        {
          ...validScenario().rules[0],
          outcomes: [
            {
              kind: "complete",
              lineageCursor: { kind: "initialSubmission", submissionId: "missing" },
            },
          ],
        },
      ],
    }),
    code: "MISSING_LINEAGE_CURSOR_TARGET",
  },
  {
    name: "a forward scripted lineage target",
    scenario: validScenario({
      rules: [
        {
          ...validScenario().rules[0],
          id: "first",
          outcomes: [
            {
              kind: "complete",
              lineageCursor: {
                kind: "scriptedOutcome",
                ruleId: "second",
                outcomeIndex: 0,
              },
            },
          ],
        },
        {
          ...validScenario().rules[0],
          id: "second",
          match: { kind: "workType", workType: "fulfillment" },
        },
      ],
    }),
    code: "FORWARD_LINEAGE_CURSOR",
  },
  {
    name: "a cyclic scripted lineage target",
    scenario: validScenario({
      rules: [
        {
          ...validScenario().rules[0],
          id: "first",
          outcomes: [
            {
              kind: "complete",
              lineageCursor: {
                kind: "scriptedOutcome",
                ruleId: "second",
                outcomeIndex: 0,
              },
            },
          ],
        },
        {
          ...validScenario().rules[0],
          id: "second",
          match: { kind: "workType", workType: "fulfillment" },
          outcomes: [
            {
              kind: "complete",
              lineageCursor: {
                kind: "scriptedOutcome",
                ruleId: "first",
                outcomeIndex: 0,
              },
            },
          ],
        },
      ],
    }),
    code: "CYCLIC_LINEAGE_CURSOR",
  },
  {
    name: "an incompatible reject lineage target",
    scenario: validScenario({
      rules: [
        {
          ...validScenario().rules[0],
          id: "first",
          outcomes: [{ kind: "reject", reason: "not complete" }],
        },
        {
          ...validScenario().rules[0],
          id: "second",
          match: { kind: "workType", workType: "fulfillment" },
          outcomes: [
            {
              kind: "complete",
              lineageCursor: {
                kind: "scriptedOutcome",
                ruleId: "first",
                outcomeIndex: 0,
              },
            },
          ],
        },
      ],
    }),
    code: "INCOMPATIBLE_LINEAGE_CURSOR",
  },
]) {
  test(`rejects ${invalidCase.name} before emulator activity`, () => {
    const result = parseEmulatorScenario(
      invalidCase.scenario,
      supportedFactory({
        workTypes: [{ name: "checkout" }, { name: "fulfillment" }],
      }),
    );

    assert.equal(result.success, false);
    assert.ok(result.diagnostics.some(({ code }) => code === invalidCase.code));
  });
}

test("reports only provably shadowed rules and resolves the first match", () => {
  const scenario = validScenario({
    initialSubmissions: [{ id: "checkout-1", workType: "checkout" }],
    rules: [
      {
        id: "checkout-first",
        match: { kind: "workType", workType: "checkout" },
        outcomes: [{ kind: "complete", output: { winner: "first" } }],
        exhaustionBehavior: { kind: "repeatLast" },
      },
      {
        id: "checkout-later",
        match: { kind: "submissionId", submissionId: "checkout-1" },
        outcomes: [{ kind: "reject", reason: "unreachable" }],
        exhaustionBehavior: { kind: "reject", reason: "unreachable" },
      },
      {
        id: "all-later",
        match: { kind: "all" },
        outcomes: [{ kind: "reject", reason: "fallback" }],
        exhaustionBehavior: { kind: "useUnmatchedBehavior" },
      },
    ],
  });

  const parsed = parseEmulatorScenario(scenario, supportedFactory());
  assert.equal(parsed.success, false);
  assert.deepEqual(
    parsed.diagnostics
      .filter(({ code }) => code === "SHADOWED_RULE")
      .map(({ path }) => path),
    ["/rules/1"],
  );
  assert.equal(
    selectEmulatorRule(scenario, { id: "checkout-1", workType: "checkout" }).id,
    "checkout-first",
  );
});

test("resolves finite outcomes, explicit exhaustion, and unmatched behavior", () => {
  const scenario = validScenario({
    rules: [
      {
        id: "checkout",
        match: { kind: "workType", workType: "checkout" },
        outcomes: [
          { kind: "complete", output: { attempt: 1 } },
          { kind: "reject", reason: "second attempt" },
        ],
        exhaustionBehavior: { kind: "repeatLast" },
      },
    ],
    unmatchedBehavior: { kind: "reject", reason: "no matching rule" },
  });

  assert.deepEqual(
    resolveEmulatorScenarioResult(
      scenario,
      { id: "checkout-1", workType: "checkout" },
      0,
    ),
    {
      kind: "outcome",
      rule: scenario.rules[0],
      outcome: scenario.rules[0].outcomes[0],
    },
  );
  assert.deepEqual(
    resolveEmulatorScenarioResult(
      scenario,
      { id: "checkout-1", workType: "checkout" },
      2,
    ).outcome,
    scenario.rules[0].outcomes[1],
  );
  assert.deepEqual(
    resolveEmulatorScenarioResult(
      scenario,
      { id: "other-1", workType: "other" },
      0,
    ),
    { kind: "unmatched", behavior: scenario.unmatchedBehavior },
  );
  assert.throws(
    () =>
      resolveEmulatorScenarioResult(
        scenario,
        { id: "checkout-1", workType: "checkout" },
        -1,
      ),
    /zero-based safe integer/,
  );

  const unmatchedAfterExhaustion = {
    ...scenario,
    rules: [
      {
        ...scenario.rules[0],
        exhaustionBehavior: { kind: "useUnmatchedBehavior" },
      },
    ],
  };
  assert.deepEqual(
    resolveEmulatorScenarioResult(
      unmatchedAfterExhaustion,
      { id: "checkout-1", workType: "checkout" },
      2,
    ),
    { kind: "unmatched", behavior: unmatchedAfterExhaustion.unmatchedBehavior },
  );

  const rejectedAfterExhaustion = {
    ...scenario,
    rules: [
      {
        ...scenario.rules[0],
        exhaustionBehavior: { kind: "reject", reason: "script exhausted" },
      },
    ],
  };
  assert.deepEqual(
    resolveEmulatorScenarioResult(
      rejectedAfterExhaustion,
      { id: "checkout-1", workType: "checkout" },
      2,
    ),
    {
      kind: "exhausted",
      rule: rejectedAfterExhaustion.rules[0],
      behavior: { kind: "reject", reason: "script exhausted" },
    },
  );
});

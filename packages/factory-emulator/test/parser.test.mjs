import assert from "node:assert/strict";
import test from "node:test";
import {
  parseEmulatorScenario,
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

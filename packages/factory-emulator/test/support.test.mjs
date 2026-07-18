import assert from "node:assert/strict";
import test from "node:test";
import {
  emulatorScenarioExamples,
  inspectEmulatorSupport,
  parseEmulatorScenario,
  resolveEmulatorScenarioResult,
  selectEmulatorRule,
  SUPPORTED_SCENARIO_VERSION,
} from "@you-agent-factory/factory-emulator";

test("inspection reports the parser-supported v1 scenario and Factory subset", () => {
  const support = inspectEmulatorSupport();

  assert.equal(support.scenarioVersion, SUPPORTED_SCENARIO_VERSION);
  assert.deepEqual(support.ruleMatchers, ["all", "workType", "submissionId"]);
  assert.deepEqual(support.outcomeVariants, ["complete", "reject"]);
  assert.deepEqual(support.exhaustionBehaviors, [
    "repeatLast",
    "useUnmatchedBehavior",
    "reject",
  ]);
  assert.deepEqual(support.unmatchedBehaviors, ["ignore", "reject"]);
  assert.equal(support.activityLabel.maximumLength, 120);
  assert.equal(support.activityLabel.transient, true);
  assert.equal(support.activityLabel.canonicalFactoryEventField, false);

  const rejected = parseEmulatorScenario(
    emulatorScenarioExamples[0].scenario,
    { ...emulatorScenarioExamples[0].factory, resources: [{ name: "quota" }] },
  );
  assert.equal(rejected.success, false);
  assert.ok(
    rejected.diagnostics.some(
      ({ code, path }) => code === "UNSUPPORTED_FACTORY_CAPABILITY" && path === "/resources",
    ),
  );
});

test("every published example parses through the public API", () => {
  for (const example of emulatorScenarioExamples) {
    const result = parseEmulatorScenario(example.scenario, example.factory);
    assert.deepEqual(result, {
      success: true,
      scenario: example.scenario,
      factory: example.factory,
    }, example.name);
  }
});

test("the multi-rule example demonstrates ordered matching and deterministic exhaustion", () => {
  const example = emulatorScenarioExamples.find(({ name }) => name === "multi-rule-lineage");
  assert.ok(example);

  assert.equal(
    selectEmulatorRule(example.scenario, { id: "checkout-1", workType: "checkout" }).id,
    "priority-checkout",
  );
  assert.equal(
    selectEmulatorRule(example.scenario, { id: "checkout-2", workType: "checkout" }).id,
    "remaining-checkout",
  );
  assert.deepEqual(
    resolveEmulatorScenarioResult(
      example.scenario,
      { id: "checkout-1", workType: "checkout" },
      1,
    ),
    { kind: "unmatched", behavior: example.scenario.unmatchedBehavior },
  );
});

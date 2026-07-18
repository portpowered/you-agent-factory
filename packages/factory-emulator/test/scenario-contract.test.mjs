import assert from "node:assert/strict";
import test from "node:test";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import publishedSchema from "@you-agent-factory/factory-emulator/schema" with {
  type: "json",
};
import {
  scenarioSchema,
  SUPPORTED_SCENARIO_VERSION,
} from "@you-agent-factory/factory-emulator";

const ajv = new Ajv2020({ allErrors: true, strict: false });
addFormats(ajv);
const validate = ajv.compile(publishedSchema);

function validScenario(overrides = {}) {
  return {
    version: SUPPORTED_SCENARIO_VERSION,
    id: "deterministic-checkout",
    seed: "seed-0001",
    startAt: "2026-07-18T07:30:00Z",
    initialSubmissions: [{ id: "submission-1", workType: "checkout" }],
    rules: [
      {
        id: "complete-checkout",
        match: { kind: "workType", workType: "checkout" },
        outcomes: [
          {
            kind: "complete",
            durationMs: 25,
            lineageCursor: {
              kind: "initialSubmission",
              submissionId: "submission-1",
            },
          },
        ],
        exhaustionBehavior: { kind: "repeatLast" },
      },
    ],
    unmatchedBehavior: { kind: "ignore" },
    activityLabel: "Running deterministic checkout",
    ...overrides,
  };
}

test("published schema and stable module export the v1 contract", () => {
  assert.equal(publishedSchema.$id, scenarioSchema.$id);
  assert.equal(
    publishedSchema.properties.version.const,
    SUPPORTED_SCENARIO_VERSION,
  );
  assert.equal(publishedSchema.additionalProperties, false);
});

test("published schema accepts a representative deterministic scenario", () => {
  assert.equal(validate(validScenario()), true, JSON.stringify(validate.errors));
});

for (const invalidCase of [
  {
    name: "missing deterministic metadata",
    document: (() => {
      const scenario = validScenario();
      delete scenario.seed;
      return scenario;
    })(),
  },
  {
    name: "a non-UTC startAt",
    document: validScenario({ startAt: "2026-07-18T00:30:00-07:00" }),
  },
  {
    name: "an unsupported version",
    document: validScenario({ version: "you-agent-factory.emulator.scenario.v2" }),
  },
  {
    name: "an invalid exhaustion behavior variant",
    document: validScenario({
      rules: [
        {
          ...validScenario().rules[0],
          exhaustionBehavior: { kind: "fallThrough" },
        },
      ],
    }),
  },
  {
    name: "unknown contract fields",
    document: validScenario({ unexpected: true }),
  },
  {
    name: "a negative virtual outcome duration",
    document: validScenario({
      rules: [{
        ...validScenario().rules[0],
        outcomes: [{ kind: "complete", durationMs: -1 }],
      }],
    }),
  },
  {
    name: "a fractional virtual outcome duration",
    document: validScenario({
      rules: [{
        ...validScenario().rules[0],
        outcomes: [{ kind: "complete", durationMs: 0.5 }],
      }],
    }),
  },
]) {
  test(`published schema rejects ${invalidCase.name}`, () => {
    assert.equal(validate(invalidCase.document), false);
  });
}

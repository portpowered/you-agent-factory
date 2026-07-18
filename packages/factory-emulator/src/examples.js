import { SUPPORTED_SCENARIO_VERSION } from "./generated/scenario-schema.js";

/** Published, parser-validated documents for consumers starting an emulator host. */
export const emulatorScenarioExamples = Object.freeze([
  {
    name: "minimal",
    factory: {
      name: "minimal-emulator-factory",
      workTypes: [{ name: "checkout" }],
      workstations: [{ name: "complete", behavior: "STANDARD" }],
    },
    scenario: {
      version: SUPPORTED_SCENARIO_VERSION,
      id: "minimal-checkout",
      seed: "minimal-checkout-seed",
      startAt: "2026-07-18T08:00:00Z",
      rules: [{
        id: "complete-checkout",
        match: { kind: "workType", workType: "checkout" },
        outcomes: [{ kind: "complete" }],
        exhaustionBehavior: { kind: "repeatLast" },
      }],
      unmatchedBehavior: { kind: "ignore" },
    },
  },
  {
    name: "multi-rule-lineage",
    factory: {
      name: "multi-rule-emulator-factory",
      workTypes: [{ name: "checkout" }, { name: "fulfillment" }, { name: "other" }],
      workstations: [{ name: "complete", behavior: "STANDARD" }],
    },
    scenario: {
      version: SUPPORTED_SCENARIO_VERSION,
      id: "checkout-and-fulfillment",
      seed: "checkout-and-fulfillment-seed",
      startAt: "2026-07-18T08:30:00Z",
      activityLabel: "Emulating deterministic checkout",
      initialSubmissions: [
        { id: "checkout-1", workType: "checkout" },
        { id: "fulfillment-1", workType: "fulfillment" },
      ],
      rules: [
        {
          id: "priority-checkout",
          match: { kind: "submissionId", submissionId: "checkout-1" },
          outcomes: [{
            kind: "complete",
            lineageCursor: { kind: "initialSubmission", submissionId: "checkout-1" },
          }],
          exhaustionBehavior: { kind: "useUnmatchedBehavior" },
        },
        {
          id: "remaining-checkout",
          match: { kind: "workType", workType: "checkout" },
          outcomes: [{ kind: "complete" }],
          exhaustionBehavior: { kind: "repeatLast" },
        },
        {
          id: "fulfillment-from-checkout",
          match: { kind: "workType", workType: "fulfillment" },
          outcomes: [{
            kind: "complete",
            lineageCursor: { kind: "scriptedOutcome", ruleId: "priority-checkout", outcomeIndex: 0 },
          }],
          exhaustionBehavior: { kind: "useUnmatchedBehavior" },
        },
      ],
      unmatchedBehavior: { kind: "reject", reason: "no scripted match" },
    },
  },
]);

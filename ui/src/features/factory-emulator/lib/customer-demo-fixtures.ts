import type { FactoryDefinition } from "@you-agent-factory/client";
import type { FactoryEmulatorScenario } from "@you-agent-factory/factory-emulator";

import { getFactoryEmulatorMessages } from "../messages/factory-emulator";

const activityLabels = getFactoryEmulatorMessages("en").demos.activityLabels;

export interface CustomerFactoryEmulatorDemoFixture {
  readonly id: "success" | "repeat-review-failure";
  readonly factory: FactoryDefinition;
  readonly scenario: FactoryEmulatorScenario;
}

const successFactory = {
  name: "customer-demo-success",
  orchestrator: { kind: "PETRI" },
  workTypes: [
    {
      handlingBehavior: ["DEFAULT"],
      name: "task",
      states: [
        { name: "ready", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
        { name: "failed", type: "FAILED" },
      ],
    },
  ],
  workers: [{ name: "execute-worker", type: "AGENT_WORKER" }],
  workstations: [
    {
      behavior: "STANDARD",
      inputs: [{ workType: "task", state: "ready" }],
      name: "Execute",
      onFailure: [{ workType: "task", state: "failed" }],
      outputs: [{ workType: "task", state: "done" }],
      type: "AGENT_RUN",
      worker: "execute-worker",
    },
  ],
} satisfies FactoryDefinition;

const successScenario = {
  schemaVersion: "factory-emulator-scenario/v1",
  id: "customer-demo-success-scenario",
  factory: { name: successFactory.name },
  seed: "customer-demo-success-seed-v1",
  startAt: "2026-07-19T16:00:00.000Z",
  initialSubmissions: [
    {
      input: "Prepare a concise launch summary.",
      name: "launch-summary",
      state: "ready",
      workType: "task",
    },
  ],
  rules: [
    {
      cursor: { input: "rootWorkId", scope: "lineage" },
      exhaustion: "fail",
      id: "execute-success",
      outcomes: [
        {
          activityLabel: activityLabels.prepare,
          durationMs: 1_500,
          output: "Launch summary ready.",
          result: "accepted",
        },
      ],
      selector: { workstation: "Execute", worker: "execute-worker" },
    },
  ],
  unmatched: { behavior: "error" },
} satisfies FactoryEmulatorScenario;

const repeatReviewFailureFactory = {
  name: "customer-demo-repeat-review-failure",
  orchestrator: { kind: "PETRI" },
  workTypes: [
    {
      handlingBehavior: ["DEFAULT"],
      name: "task",
      states: [
        { name: "ready", type: "INITIAL" },
        { name: "review", type: "PROCESSING" },
        { name: "done", type: "TERMINAL" },
        { name: "failed", type: "FAILED" },
      ],
    },
  ],
  workers: [
    { name: "execute-worker", type: "AGENT_WORKER" },
    { name: "review-worker", type: "AGENT_WORKER" },
  ],
  workstations: [
    {
      behavior: "REPEATER",
      inputs: [{ workType: "task", state: "ready" }],
      name: "Execute",
      onContinue: [{ workType: "task", state: "ready" }],
      onFailure: [{ workType: "task", state: "failed" }],
      outputs: [{ workType: "task", state: "review" }],
      type: "AGENT_RUN",
      worker: "execute-worker",
    },
    {
      behavior: "STANDARD",
      inputs: [{ workType: "task", state: "review" }],
      name: "Review",
      onFailure: [{ workType: "task", state: "failed" }],
      onRejection: [{ workType: "task", state: "ready" }],
      outputs: [{ workType: "task", state: "done" }],
      type: "AGENT_RUN",
      worker: "review-worker",
    },
  ],
} satisfies FactoryDefinition;

const repeatReviewFailureScenario = {
  schemaVersion: "factory-emulator-scenario/v1",
  id: "customer-demo-repeat-review-failure-scenario",
  factory: { name: repeatReviewFailureFactory.name },
  seed: "customer-demo-repeat-review-failure-seed-v1",
  startAt: "2026-07-19T17:00:00.000Z",
  initialSubmissions: [
    {
      input: "Draft and review a customer launch plan.",
      name: "customer-launch-plan",
      state: "ready",
      workType: "task",
    },
  ],
  rules: [
    {
      cursor: { input: "rootWorkId", scope: "lineage" },
      exhaustion: "fail",
      id: "execute-repeat-sequence",
      outcomes: [
        {
          activityLabel: activityLabels.draft,
          durationMs: 1_500,
          result: "continued",
        },
        {
          activityLabel: activityLabels.revise,
          durationMs: 1_500,
          output: "First draft ready for review.",
          result: "accepted",
        },
        {
          activityLabel: activityLabels.polish,
          durationMs: 1_500,
          output: "Revised draft ready for review.",
          result: "accepted",
        },
      ],
      selector: { workstation: "Execute", worker: "execute-worker" },
    },
    {
      cursor: { input: "rootWorkId", scope: "lineage" },
      exhaustion: "fail",
      id: "review-rework-failure-sequence",
      outcomes: [
        {
          activityLabel: activityLabels.firstReview,
          durationMs: 1_000,
          feedback: "Clarify the rollout and retry.",
          result: "rejected",
        },
        {
          activityLabel: activityLabels.finalReview,
          durationMs: 1_000,
          error: "The revised plan did not pass final review.",
          result: "failed",
        },
      ],
      selector: { workstation: "Review", worker: "review-worker" },
    },
  ],
  unmatched: { behavior: "error" },
} satisfies FactoryEmulatorScenario;

export const customerFactoryEmulatorDemoFixtures = {
  success: {
    id: "success",
    factory: successFactory,
    scenario: successScenario,
  },
  repeatReviewFailure: {
    id: "repeat-review-failure",
    factory: repeatReviewFailureFactory,
    scenario: repeatReviewFailureScenario,
  },
} as const satisfies Readonly<
  Record<string, CustomerFactoryEmulatorDemoFixture>
>;

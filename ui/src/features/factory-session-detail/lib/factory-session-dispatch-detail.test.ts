import type { FactoryDispatch } from "../../../api/factory-sessions/dispatch-detail";
import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { describe, expect, it } from "vitest";
import { normalizeFactorySessionDispatchDetail } from "./factory-session-dispatch-detail";

const successfulDispatchFixture = {
  artifactIds: ["artifact-final-1", "artifact-log-2"],
  attempt: 2,
  dispatchKind: "JAVASCRIPT_AGENT",
  id: "dispatch-success-1",
  javascript: {
    executionMode: " live ",
    taskKind: "AGENT",
    taskLabel: " Draft response ",
  },
  label: " Draft response ",
  model: " gpt-5.5 ",
  orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
  phase: " deliver ",
  promptDigest: " sha256:prompt-1 ",
  provider: " openai ",
  providerSessionRefs: [
    {
      id: "sess_codex_1",
      kind: "session_id",
      provider: "codex",
    },
  ],
  relatedWorkIds: ["work-alpha", "work-beta"],
  runnerId: " runner-web-1 ",
  schemaDigest: " sha256:schema-1 ",
  sessionId: "dur-sess-js-success-1",
  status: "COMPLETED",
  statusTransitions: ["QUEUED", "RUNNING", "COMPLETED"],
  usage: {
    costUsd: 0.21,
    durationMillis: 4400,
    inputTokens: 120,
    outputTokens: 80,
    retryCount: 1,
    totalTokens: 200,
  },
  warnings: [
    {
      code: "DISPATCH_WARNING",
      message: "Token budget was nearly exhausted.",
    },
  ],
} as FactoryDispatch & {
  statusTransitions: string[];
  javascript: FactoryDispatch["javascript"] & {
    executionMode: string;
  };
};

const failedDispatchFixture = {
  artifactIds: [],
  dispatchKind: "JAVASCRIPT_VERIFY",
  failureDetail: {
    errorClass: " verification_error ",
    message: " Expected release manifest checksum. ",
    reason: " VERIFY_ASSERTION_FAILED ",
  },
  id: "dispatch-failed-1",
  javascript: {
    executionMode: " live ",
    taskKind: "VERIFY",
  },
  label: " verify ",
  orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
  providerSessionRefs: [],
  relatedWorkIds: [],
  runnerId: "   ",
  sessionId: "dur-sess-js-failed-1",
  status: "FAILED",
  statusTransitions: ["QUEUED", "RUNNING", "FAILED"],
} as FactoryDispatch & {
  statusTransitions: string[];
  javascript: FactoryDispatch["javascript"] & {
    executionMode: string;
  };
};

describe("normalizeFactorySessionDispatchDetail", () => {
  it("maps a successful durable JavaScript dispatch into a website drilldown model", () => {
    expect(
      normalizeFactorySessionDispatchDetail(successfulDispatchFixture),
    ).toEqual({
      artifactLinks: [
        {
          href: "/factory-sessions/dur-sess-js-success-1/artifacts/artifact-final-1",
          id: "artifact-final-1",
        },
        {
          href: "/factory-sessions/dur-sess-js-success-1/artifacts/artifact-log-2",
          id: "artifact-log-2",
        },
      ],
      attempt: 2,
      dispatchID: "dispatch-success-1",
      dispatchKind: "JAVASCRIPT_AGENT",
      javascript: {
        executionMode: "live",
        taskKind: "AGENT",
        taskLabel: "Draft response",
      },
      label: "Draft response",
      model: "gpt-5.5",
      orchestratorKind: "JAVASCRIPT",
      phase: "deliver",
      promptDigest: "sha256:prompt-1",
      provider: "openai",
      providerSessionRefs: [
        {
          id: "sess_codex_1",
          kind: "session_id",
          provider: "codex",
        },
      ],
      relatedWorkIDs: ["work-alpha", "work-beta"],
      runnerID: "runner-web-1",
      schemaDigest: "sha256:schema-1",
      sessionID: "dur-sess-js-success-1",
      status: "COMPLETED",
      statusHistory: ["QUEUED", "RUNNING", "COMPLETED"],
      usage: {
        costUsd: 0.21,
        durationMillis: 4400,
        inputTokens: 120,
        outputTokens: 80,
        retryCount: 1,
        totalTokens: 200,
      },
      warnings: [
        {
          code: "DISPATCH_WARNING",
          message: "Token budget was nearly exhausted.",
        },
      ],
    });
  });

  it("maps a failed durable dispatch with typed failure detail and omits blank optional fields", () => {
    expect(
      normalizeFactorySessionDispatchDetail(failedDispatchFixture),
    ).toEqual({
      artifactLinks: [],
      dispatchID: "dispatch-failed-1",
      dispatchKind: "JAVASCRIPT_VERIFY",
      failureDetail: {
        errorClass: "verification_error",
        message: "Expected release manifest checksum.",
        reason: "VERIFY_ASSERTION_FAILED",
      },
      javascript: {
        executionMode: "live",
        taskKind: "VERIFY",
        taskLabel: undefined,
      },
      label: "verify",
      model: undefined,
      orchestratorKind: "JAVASCRIPT",
      phase: undefined,
      promptDigest: undefined,
      provider: undefined,
      providerSessionRefs: [],
      relatedWorkIDs: [],
      runnerID: undefined,
      schemaDigest: undefined,
      sessionID: "dur-sess-js-failed-1",
      status: "FAILED",
      statusHistory: ["QUEUED", "RUNNING", "FAILED"],
      usage: undefined,
      warnings: [],
    });
  });
});

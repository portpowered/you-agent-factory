// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: dispatch drilldown projection cases share one normalization harness.

import { describe, expect, it, vi } from "vitest";
import {
  type FactoryDispatch,
  getFactorySessionDispatchDetail,
} from "../../../api/factory-sessions/dispatch-detail";
import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
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
} satisfies FactoryDispatch;

const minimalDispatchFixture = {
  dispatchKind: "JAVASCRIPT_AGENT",
  id: "dispatch-minimal-1",
  orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
  sessionId: "dur-sess-js-minimal-1",
  status: "QUEUED",
} satisfies FactoryDispatch;

const warningDispatchFixture = {
  artifactIds: ["artifact-warning-log"],
  dispatchKind: "JAVASCRIPT_VERIFY",
  id: "dispatch-warning-1",
  javascript: {
    executionMode: "live",
    taskKind: "VERIFY",
  },
  orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
  sessionId: "dur-sess-js-warning-1",
  status: "COMPLETED",
  warnings: [
    {
      code: "DISPATCH_WARNING",
      message: "Verification completed with non-blocking warnings.",
    },
  ],
} satisfies FactoryDispatch;

const failedDispatchFixture = {
  artifactIds: [],
  dispatchKind: "JAVASCRIPT_VERIFY",
  failureDetail: {
    message: " Expected release manifest checksum. ",
    reason: "unknown",
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
} satisfies FactoryDispatch;

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

  it("maps a minimal durable dispatch with missing optional values into empty collections", () => {
    expect(
      normalizeFactorySessionDispatchDetail(minimalDispatchFixture),
    ).toEqual({
      artifactLinks: [],
      dispatchID: "dispatch-minimal-1",
      dispatchKind: "JAVASCRIPT_AGENT",
      orchestratorKind: "JAVASCRIPT",
      providerSessionRefs: [],
      relatedWorkIDs: [],
      sessionID: "dur-sess-js-minimal-1",
      status: "QUEUED",
      statusHistory: [],
      warnings: [],
    });
  });

  it("maps a completed dispatch with warnings into a failed-or-warning drilldown model", () => {
    expect(
      normalizeFactorySessionDispatchDetail(warningDispatchFixture),
    ).toEqual({
      artifactLinks: [
        {
          href: "/factory-sessions/dur-sess-js-warning-1/artifacts/artifact-warning-log",
          id: "artifact-warning-log",
        },
      ],
      dispatchID: "dispatch-warning-1",
      dispatchKind: "JAVASCRIPT_VERIFY",
      javascript: {
        executionMode: "live",
        taskKind: "VERIFY",
        taskLabel: undefined,
      },
      orchestratorKind: "JAVASCRIPT",
      providerSessionRefs: [],
      relatedWorkIDs: [],
      sessionID: "dur-sess-js-warning-1",
      status: "COMPLETED",
      statusHistory: [],
      warnings: [
        {
          code: "DISPATCH_WARNING",
          message: "Verification completed with non-blocking warnings.",
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
        message: "Expected release manifest checksum.",
        reason: "unknown",
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

describe("factory session dispatch drilldown regression and scope", () => {
  it("reads per-dispatch detail from the existing durable dispatch detail route", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(successfulDispatchFixture), {
        headers: { "Content-Type": "application/json" },
        status: 200,
      }),
    );

    await getFactorySessionDispatchDetail(
      {
        dispatch_id: "dispatch-success-1",
        session_id: "dur-sess-js-success-1",
      },
      { fetch },
    );

    expect(fetch).toHaveBeenCalledTimes(1);
    expect(String(fetch.mock.calls[0]?.[0])).toContain(
      "/factory-sessions/dur-sess-js-success-1/dispatches/dispatch-success-1",
    );
    expect(String(fetch.mock.calls[0]?.[0])).not.toMatch(
      /\/dynamic-workflow|\/workflow-runs/i,
    );
  });

  it("projects successful and failed dispatches with public Factory Session vocabulary", () => {
    const failedDispatchFixture = {
      dispatchKind: "JAVASCRIPT_VERIFY",
      failureDetail: {
        message: "Expected release manifest checksum.",
        reason: "unknown",
      },
      id: "dispatch-failed-1",
      orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
      sessionId: "dur-sess-js-failed-1",
      status: "FAILED",
    } satisfies FactoryDispatch;

    const success = normalizeFactorySessionDispatchDetail(
      successfulDispatchFixture,
    );
    const failed = normalizeFactorySessionDispatchDetail(failedDispatchFixture);

    expect(success).toMatchObject({
      dispatchID: "dispatch-success-1",
      providerSessionRefs: successfulDispatchFixture.providerSessionRefs,
      sessionID: "dur-sess-js-success-1",
      status: "COMPLETED",
    });
    expect(success.artifactLinks.map((link) => link.id)).toEqual([
      "artifact-final-1",
      "artifact-log-2",
    ]);
    expect(failed).toMatchObject({
      dispatchID: "dispatch-failed-1",
      failureDetail: {
        message: "Expected release manifest checksum.",
        reason: "unknown",
      },
      sessionID: "dur-sess-js-failed-1",
      status: "FAILED",
    });

    const serialized = JSON.stringify({ failed, success });
    expect(serialized).not.toContain("DynamicWorkflowRun");
    expect(serialized).not.toContain("workflowRun");
  });
});

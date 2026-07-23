// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: dispatch detail content states share one render harness.
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { FactoryDispatch } from "../../../../api/factory-sessions/dispatch-detail";
import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import type { FactorySessionDispatchDrilldownModel } from "../../lib/factory-session-dispatch-detail";
import { normalizeFactorySessionDispatchDetail } from "../../lib/factory-session-dispatch-detail";
import { DispatchDetailContent } from "./dispatch-detail-content";

const successfulDispatchFixture = {
  artifactIds: ["artifact-final-1", "artifact-log-2"],
  attempt: 2,
  dispatchKind: "JAVASCRIPT_AGENT",
  id: "dispatch-success-1",
  javascript: {
    executionMode: "live",
    taskKind: "AGENT",
    taskLabel: "Draft response",
  },
  label: "Draft response",
  model: "gpt-5.5",
  orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
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
  relatedWorkIds: ["work-alpha", "work-beta"],
  runnerId: "runner-web-1",
  schemaDigest: "sha256:schema-1",
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
} satisfies FactoryDispatch;

const minimalDispatchFixture = {
  dispatchKind: "JAVASCRIPT_AGENT",
  id: "dispatch-minimal-1",
  orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
  sessionId: "dur-sess-js-minimal-1",
  status: "QUEUED",
} satisfies FactoryDispatch;

describe("DispatchDetailContent", () => {
  it("renders successful dispatch status, execution metadata, provider sessions, and artifact links", () => {
    const data = normalizeFactorySessionDispatchDetail(
      successfulDispatchFixture,
    );

    render(<DispatchDetailContent data={data} />);

    expect(screen.getAllByText("COMPLETED").length).toBeGreaterThan(0);
    expect(screen.getByText("live")).toBeTruthy();
    expect(screen.getByText("AGENT")).toBeTruthy();
    expect(screen.getAllByText("Draft response").length).toBeGreaterThan(0);
    expect(screen.getByText("Provider sessions")).toBeTruthy();
    expect(screen.getByText("session_id · sess_codex_1")).toBeTruthy();
    expect(screen.getByText("codex")).toBeTruthy();
    expect(screen.getByRole("link", { name: "artifact-final-1" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "artifact-log-2" })).toBeTruthy();
    expect(screen.getByText("QUEUED")).toBeTruthy();
    expect(screen.getByText("RUNNING")).toBeTruthy();
    expect(screen.getByText("work-alpha")).toBeTruthy();
    expect(screen.getByText("work-beta")).toBeTruthy();
  });

  it("omits optional sections when values are unavailable without showing error states", () => {
    const data = normalizeFactorySessionDispatchDetail(minimalDispatchFixture);

    render(<DispatchDetailContent data={data} />);

    expect(screen.getByText("QUEUED")).toBeTruthy();
    expect(screen.getByText("JAVASCRIPT_AGENT")).toBeTruthy();
    expect(screen.queryByText("JavaScript task")).toBeNull();
    expect(screen.queryByText("Provider sessions")).toBeNull();
    expect(screen.queryByText("Dispatch artifacts")).toBeNull();
    expect(screen.queryByText("Failure detail")).toBeNull();
    expect(screen.queryByText("Usage")).toBeNull();
    expect(screen.queryByText("Status history")).toBeNull();
    expect(screen.queryByText("Related work")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("renders artifact links without body preview or lifecycle controls", () => {
    const data: FactorySessionDispatchDrilldownModel = {
      artifactLinks: [
        {
          href: "/factory-sessions/session-1/artifacts/artifact-1",
          id: "artifact-1",
        },
      ],
      dispatchID: "dispatch-1",
      dispatchKind: "JAVASCRIPT_AGENT",
      orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
      providerSessionRefs: [],
      relatedWorkIDs: [],
      sessionID: "session-1",
      status: "COMPLETED",
      statusHistory: [],
      warnings: [],
    };

    render(<DispatchDetailContent data={data} />);

    const artifactLink = screen.getByRole("link", { name: "artifact-1" });
    expect(artifactLink.getAttribute("href")).toBe(
      "/factory-sessions/session-1/artifacts/artifact-1",
    );
    expect(screen.queryByText("preview")).toBeNull();
    expect(screen.queryByRole("button", { name: /lifecycle/i })).toBeNull();
  });

  it("renders failed dispatch status treatment and typed failure detail", () => {
    const data = normalizeFactorySessionDispatchDetail({
      dispatchKind: "JAVASCRIPT_VERIFY",
      failureDetail: {
        message: "Expected release manifest checksum.",
        reason: "VERIFY_ASSERTION_FAILED",
      },
      id: "dispatch-failed-1",
      javascript: {
        executionMode: "live",
        taskKind: "VERIFY",
        taskLabel: "Verify docs",
      },
      orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
      sessionId: "dur-sess-js-failed-1",
      status: "FAILED",
      statusTransitions: ["QUEUED", "RUNNING", "FAILED"],
    } satisfies FactoryDispatch);

    render(<DispatchDetailContent data={data} />);

    expect(screen.getAllByText("FAILED").length).toBeGreaterThan(0);
    expect(screen.getByText("Failure detail")).toBeTruthy();
    expect(screen.getByText("VERIFY_ASSERTION_FAILED")).toBeTruthy();
    expect(
      screen.getByText("Expected release manifest checksum."),
    ).toBeTruthy();
    expect(screen.queryByText("Error class")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("renders warning dispatch status treatment and typed warning detail", () => {
    const data = normalizeFactorySessionDispatchDetail({
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
    } satisfies FactoryDispatch);

    render(<DispatchDetailContent data={data} />);

    expect(screen.getAllByText("COMPLETED").length).toBeGreaterThan(0);
    expect(screen.getByText("Dispatch warnings")).toBeTruthy();
    expect(screen.getByText("DISPATCH_WARNING")).toBeTruthy();
    expect(
      screen.getByText("Verification completed with non-blocking warnings."),
    ).toBeTruthy();
    expect(screen.queryByText("Failure detail")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

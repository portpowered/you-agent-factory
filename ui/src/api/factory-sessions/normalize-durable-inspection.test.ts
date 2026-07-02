import { FactoryOrchestratorKind } from "../generated/openapi";
import {
  buildFailedBridgedChildDispatchSummary,
  buildSuccessfulLiveProviderDispatchSummary,
  failedBridgedChildSessionID,
  successfulLiveProviderSessionID,
} from "../../testing/factory-session-live-provider-inspection-fixtures";
import {
  dispatchSummariesToFactoryDispatches,
  durableSupplementalReadPlan,
  resultSurfacesFromDurableReadModel,
  resultSurfacesFromDurableResult,
  shouldFetchDurableDispatches,
  shouldFetchDurableFinalResults,
  shouldFetchDurablePartialResults,
} from "./normalize-durable-inspection";

describe("normalize durable inspection helpers", () => {
  it("maps durable dispatch summaries into shared FactoryDispatch rows", () => {
    expect(
      dispatchSummariesToFactoryDispatches(
        successfulLiveProviderSessionID,
        FactoryOrchestratorKind.JAVASCRIPT,
        [buildSuccessfulLiveProviderDispatchSummary()],
      ),
    ).toEqual([
      expect.objectContaining({
        artifactIds: ["art-js-success-001"],
        id: "disp-js-success-002",
        javascript: {
          executionMode: "live",
          taskKind: "VERIFY",
          taskLabel: "verify-docs",
        },
        label: "verify-docs",
        providerSessionRefs: [
          {
            id: "resp-docs-refresh-001",
            kind: "session_id",
            provider: "codex",
          },
        ],
        sessionId: successfulLiveProviderSessionID,
        status: "COMPLETED",
      }),
    ]);
  });

  it("maps failed bridged-child dispatch summaries with provider-session refs and typed failure detail", () => {
    expect(
      dispatchSummariesToFactoryDispatches(
        failedBridgedChildSessionID,
        FactoryOrchestratorKind.JAVASCRIPT,
        [buildFailedBridgedChildDispatchSummary()],
      ),
    ).toEqual([
      expect.objectContaining({
        failureDetail: {
          errorClass: "verification_error",
          message: "Expected release manifest checksum.",
          reason: "VERIFY_ASSERTION_FAILED",
        },
        id: "disp-js-fail-002",
        javascript: {
          executionMode: "live",
          taskKind: "VERIFY",
          taskLabel: "verify",
        },
        providerSessionRefs: [
          {
            id: "resp-verify-failed-001",
            kind: "session_id",
            provider: "codex",
          },
        ],
        sessionId: failedBridgedChildSessionID,
        status: "FAILED",
      }),
    ]);
  });

  it("maps durable session artifact refs into final result surfaces", () => {
    expect(
      resultSurfacesFromDurableReadModel({
        artifactRefs: [
          {
            id: "art-js-success-001",
            kind: "FINAL_RESULT",
            visibility: "PUBLIC",
          },
        ],
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        resolvedSource: {
          kind: "WORKFLOW_FILE",
          sourceRef: "workflow/.claude/workflows/docs-refresh.yaml",
        },
        resultSummary: {
          resultStatus: "FINAL",
          summary: "Documentation refresh complete.",
        },
        sessionId: "dur-sess-js-success-002",
        status: "SUCCEEDED",
      }),
    ).toEqual({
      result: {
        resultArtifactRef: {
          id: "art-js-success-001",
          kind: "FINAL_RESULT",
          visibility: "PUBLIC",
        },
        sessionId: "dur-sess-js-success-002",
        status: "FINISHED",
      },
    });
  });
});

describe("resultSurfacesFromDurableResult", () => {
  it("maps durable result reads into partial result surfaces", () => {
    expect(
      resultSurfacesFromDurableResult(
        {
          artifactRefs: [
            {
              id: "art-js-pause-001",
              kind: "FINDING",
              visibility: "PUBLIC",
            },
          ],
          mode: "partial",
          primaryResult: [
            {
              json: {
                phase: "approval",
              },
              type: "JSON",
            },
          ],
          resultStatus: "PARTIAL",
          sessionId: "dur-sess-js-paused-001",
          sessionStatus: "PAUSED",
        },
        "fallback-phase",
      ),
    ).toEqual({
      partialResult: {
        partialResultArtifactRef: {
          id: "art-js-pause-001",
          kind: "FINDING",
          visibility: "PUBLIC",
        },
        phase: "approval",
        sessionId: "dur-sess-js-paused-001",
      },
    });
  });
});

describe("shouldFetchDurablePartialResults", () => {
  it("skips durable partial-result fetch when final result is already projected", () => {
    const surfaces = resultSurfacesFromDurableReadModel({
      artifactRefs: [
        {
          id: "art-js-success-001",
          kind: "FINAL_RESULT",
          visibility: "PUBLIC",
        },
      ],
      orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
      resolvedSource: {
        kind: "WORKFLOW_FILE",
        sourceRef: "workflow/.claude/workflows/docs-refresh.yaml",
      },
      resultSummary: {
        resultStatus: "FINAL",
        summary: "Documentation refresh complete.",
      },
      sessionId: "dur-sess-js-success-002",
      status: "SUCCEEDED",
    });

    expect(
      shouldFetchDurablePartialResults({
        partialResult: surfaces.partialResult,
        result: surfaces.result,
        resultSummary: {
          resultStatus: "FINAL",
          summary: "Documentation refresh complete.",
        },
      }),
    ).toBe(false);
  });

  it("skips durable partial-result fetch for running sessions without partial result status", () => {
    expect(
      shouldFetchDurablePartialResults({
        resultSummary: undefined,
      }),
    ).toBe(false);
  });

  it("allows durable partial-result fetch when result summary is partial", () => {
    expect(
      shouldFetchDurablePartialResults({
        resultSummary: {
          resultStatus: "PARTIAL",
        },
      }),
    ).toBe(true);
  });
});

describe("durable supplemental read plan", () => {
  it("skips supplemental reads for in-flight durable JavaScript summary sessions", () => {
    expect(
      durableSupplementalReadPlan({
        durableLifecycleStatus: "RUNNING",
        progress: {
          completedDispatches: 1,
          failedDispatches: 0,
          inFlightDispatches: 1,
          totalDispatches: 3,
        },
        resultSummary: undefined,
      }),
    ).toEqual({
      fetchDispatches: false,
      fetchFinalResults: false,
      fetchPartialResults: false,
    });
  });

  it("fetches dispatch detail for terminal durable sessions with dispatch activity", () => {
    expect(
      shouldFetchDurableDispatches({
        durableLifecycleStatus: "SUCCEEDED",
        progress: {
          completedDispatches: 2,
          failedDispatches: 0,
          inFlightDispatches: 0,
          totalDispatches: 2,
        },
      }),
    ).toBe(true);
    expect(
      shouldFetchDurableFinalResults({
        durableLifecycleStatus: "SUCCEEDED",
        result: {
          resultArtifactRef: {
            id: "art-js-success-001",
            kind: "FINAL_RESULT",
            visibility: "PUBLIC",
          },
          sessionId: "dur-sess-js-success-002",
          status: "FINISHED",
        },
        resultSummary: {
          resultStatus: "FINAL",
        },
      }),
    ).toBe(false);
  });
});

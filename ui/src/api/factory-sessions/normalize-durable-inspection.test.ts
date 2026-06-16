import { FactoryOrchestratorKind } from "../generated/openapi";
import {
  dispatchSummariesToFactoryDispatches,
  resultSurfacesFromDurableReadModel,
  resultSurfacesFromDurableResult,
  shouldFetchDurablePartialResults,
} from "./normalize-durable-inspection";

describe("normalize durable inspection helpers", () => {
  it("maps durable dispatch summaries into shared FactoryDispatch rows", () => {
    expect(
      dispatchSummariesToFactoryDispatches(
        "dur-sess-js-success-002",
        FactoryOrchestratorKind.JAVASCRIPT,
        [
          {
            attempt: 1,
            dispatchKind: "JAVASCRIPT_VERIFY",
            id: "disp-js-success-002",
            label: "verify-docs",
            outputArtifactIds: ["art-js-success-001"],
            status: "COMPLETED",
            warnings: [
              {
                code: "DISPATCH_WARNING",
                message: "child output truncated for display",
              },
            ],
          },
        ],
      ),
    ).toEqual([
      expect.objectContaining({
        artifactIds: ["art-js-success-001"],
        id: "disp-js-success-002",
        label: "verify-docs",
        sessionId: "dur-sess-js-success-002",
        status: "COMPLETED",
        warnings: [
          {
            code: "DISPATCH_WARNING",
            message: "child output truncated for display",
          },
        ],
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

  it("allows durable partial-result fetch for in-flight sessions without partial surfaces", () => {
    expect(shouldFetchDurablePartialResults({ resultSummary: undefined })).toBe(
      true,
    );
  });
});

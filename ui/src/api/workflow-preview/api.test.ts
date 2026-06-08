import { WorkflowPreviewAPIError, previewWorkflow } from "./api";
import { workflowPreviewAPIErrorMessages } from "./messages";

describe("previewWorkflow", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns the shared workflow preview contract", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          valid: true,
          sourceResolution: {
            found: true,
            requestKind: "WORKFLOW_NAME",
            sourceRef: ".claude/workflows/review.js",
            sourceHash: "sha256:abc",
          },
          sourceValidationIssues: [],
          policyPreview: {
            effectivePolicy: { mode: "READ_ONLY" },
            policyHash: "sha256:policy",
            maxChildCount: 16,
            maxConcurrency: 4,
            deniedCapabilities: [],
            validationIssues: [],
          },
          resultConstraints: {
            requiresStructuredCloneableJson: true,
            artifactUriScheme: "you-artifact",
            maxEmbeddedBytes: 65536,
            rejectedValueKinds: ["function"],
          },
        }),
        {
          headers: { "Content-Type": "application/json" },
          status: 200,
          statusText: "OK",
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await previewWorkflow({
      sourceKind: "WORKFLOW_NAME",
      projectRoot: "/tmp/project",
      sourceValue: "review",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/workflow-previews"),
      expect.objectContaining({ method: "POST" }),
    );
    expect(result.valid).toBe(true);
    expect(result.sourceResolution.sourceHash).toBe("sha256:abc");
  });

  it("surfaces malformed preview responses as typed API errors", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ valid: "invalid" }), {
          headers: { "Content-Type": "application/json" },
          status: 200,
          statusText: "OK",
        }),
      ),
    );

    await expect(
      previewWorkflow({
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "review",
      }),
    ).rejects.toBeInstanceOf(WorkflowPreviewAPIError);
  });

  it("uses the rejected request message for HTTP failures", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ message: "projectRoot is required" }), {
          headers: { "Content-Type": "application/json" },
          status: 400,
          statusText: "Bad Request",
        }),
      ),
    );

    await expect(
      previewWorkflow({
        sourceKind: "WORKFLOW_NAME",
        sourceValue: "review",
      }),
    ).rejects.toMatchObject({
      message: "projectRoot is required",
      code: "BAD_REQUEST",
    });
  });

  it("uses the network message when fetch is unavailable", async () => {
    vi.stubGlobal("fetch", undefined);

    await expect(
      previewWorkflow({
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "review",
      }),
    ).rejects.toMatchObject({
      message: workflowPreviewAPIErrorMessages.emptyEnvironment,
      code: "NETWORK_ERROR",
    });
  });
});

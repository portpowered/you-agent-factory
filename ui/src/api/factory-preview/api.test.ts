// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: preview API success and error paths share one fetch stub harness.
import { FactoryPreviewAPIError, previewFactory } from "./api";
import { factoryPreviewAPIErrorMessages } from "./messages";

describe("previewFactory", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns the canonical factory preview contract", async () => {
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

    const result = await previewFactory({
      sourceKind: "WORKFLOW_NAME",
      projectRoot: "/tmp/project",
      sourceValue: "review",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/factories/preview"),
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
      previewFactory({
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "review",
      }),
    ).rejects.toBeInstanceOf(FactoryPreviewAPIError);
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
      previewFactory({
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
      previewFactory({
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "review",
      }),
    ).rejects.toMatchObject({
      message: factoryPreviewAPIErrorMessages.emptyEnvironment,
      code: "NETWORK_ERROR",
    });
  });

  it("uses the network message when fetch throws", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockRejectedValue(new Error("connection reset")),
    );

    await expect(
      previewFactory({
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "review",
      }),
    ).rejects.toMatchObject({
      message: factoryPreviewAPIErrorMessages.network,
      code: "NETWORK_ERROR",
    });
  });

  it("maps server errors without messages to the rejected request copy", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({}), {
          headers: { "Content-Type": "application/json" },
          status: 500,
          statusText: "Internal Server Error",
        }),
      ),
    );

    await expect(
      previewFactory({
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "review",
      }),
    ).rejects.toMatchObject({
      message: factoryPreviewAPIErrorMessages.rejectedRequest,
      code: "INTERNAL_ERROR",
      status: 500,
    });
  });

  it("uses an injected fetch implementation when provided", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          valid: true,
          sourceResolution: {
            found: true,
            requestKind: "WORKFLOW_NAME",
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

    await previewFactory(
      {
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "review",
      },
      { fetch: fetchMock },
    );

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/factories/preview"),
      expect.objectContaining({
        method: "POST",
        signal: undefined,
      }),
    );
  });

  it("maps blank API error messages to the rejected request copy", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ message: "   " }), {
          headers: { "Content-Type": "application/json" },
          status: 400,
          statusText: "Bad Request",
        }),
      ),
    );

    await expect(
      previewFactory({
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "review",
      }),
    ).rejects.toMatchObject({
      message: factoryPreviewAPIErrorMessages.rejectedRequest,
      code: "BAD_REQUEST",
    });
  });

  it.each([
    [
      "source resolution",
      {
        valid: true,
        sourceResolution: "invalid",
        sourceValidationIssues: [],
        policyPreview: {},
        resultConstraints: {},
      },
    ],
    [
      "validation issues",
      {
        valid: true,
        sourceResolution: {},
        sourceValidationIssues: "invalid",
        policyPreview: {},
        resultConstraints: {},
      },
    ],
    [
      "policy preview",
      {
        valid: true,
        sourceResolution: {},
        sourceValidationIssues: [],
        policyPreview: "invalid",
        resultConstraints: {},
      },
    ],
    [
      "result constraints",
      {
        valid: true,
        sourceResolution: {},
        sourceValidationIssues: [],
        policyPreview: {},
        resultConstraints: null,
      },
    ],
  ])("rejects preview payloads with malformed %s", async (_label, payload) => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify(payload), {
          headers: { "Content-Type": "application/json" },
          status: 200,
          statusText: "OK",
        }),
      ),
    );

    await expect(
      previewFactory({
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "review",
      }),
    ).rejects.toMatchObject({
      message: factoryPreviewAPIErrorMessages.invalidResponse,
      code: "INTERNAL_ERROR",
    });
  });

  it("rejects non-object preview payloads", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify("not-an-object"), {
          headers: { "Content-Type": "application/json" },
          status: 200,
          statusText: "OK",
        }),
      ),
    );

    await expect(
      previewFactory({
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "review",
      }),
    ).rejects.toMatchObject({
      message: factoryPreviewAPIErrorMessages.invalidResponse,
      code: "INTERNAL_ERROR",
    });
  });
});

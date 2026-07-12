import * as factoryPreview from "../factory-preview";
import {
  WorkflowPreviewAPIError,
  previewWorkflow,
  type WorkflowPreviewRequest,
  type WorkflowPreviewResult,
} from "./api";

describe("previewWorkflow compatibility-only wrapper parity", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("delegates compatibility-only preview calls to previewFactory", async () => {
    const previewResult: WorkflowPreviewResult = {
      valid: true,
      sourceResolution: {
        found: true,
        requestKind: "WORKFLOW_NAME",
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
    };
    const previewFactorySpy = vi
      .spyOn(factoryPreview, "previewFactory")
      .mockResolvedValue(previewResult);

    const request: WorkflowPreviewRequest = {
      sourceKind: "WORKFLOW_NAME",
      projectRoot: "/tmp/project",
      sourceValue: "review",
    };
    const signal = new AbortController().signal;
    const fetchMock = vi.fn();

    const result = await previewWorkflow(request, {
      fetch: fetchMock,
      signal,
    });

    expect(previewFactorySpy).toHaveBeenCalledWith(request, {
      fetch: fetchMock,
      signal,
    });
    expect(result).toBe(previewResult);
  });

  it("re-exports FactoryPreviewAPIError under the compatibility-only WorkflowPreview name", () => {
    expect(WorkflowPreviewAPIError).toBe(factoryPreview.FactoryPreviewAPIError);
  });

  it("preserves the canonical Factory preview rejection", async () => {
    const rejection = new factoryPreview.FactoryPreviewAPIError(
      "INVALID_REQUEST",
      "The Factory preview request is invalid.",
      400,
    );
    vi.spyOn(factoryPreview, "previewFactory").mockRejectedValue(rejection);

    await expect(
      previewWorkflow({
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "missing",
      }),
    ).rejects.toBe(rejection);
  });
});

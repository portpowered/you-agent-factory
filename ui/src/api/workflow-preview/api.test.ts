import * as factoryPreview from "../factory-preview";
import {
  WorkflowPreviewAPIError,
  previewWorkflow,
  type WorkflowPreviewRequest,
  type WorkflowPreviewResult,
} from "./api";

describe("previewWorkflow compatibility wrapper", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("delegates obsolete preview calls to previewFactory", async () => {
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

  it("re-exports FactoryPreviewAPIError under the obsolete WorkflowPreview name", () => {
    expect(WorkflowPreviewAPIError).toBe(factoryPreview.FactoryPreviewAPIError);
  });
});

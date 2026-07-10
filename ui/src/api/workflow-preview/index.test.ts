import * as factoryPreview from "../factory-preview";
import {
  WorkflowPreviewAPIError,
  previewWorkflow,
  workflowPreviewAPIErrorMessages,
} from "./index";

describe("workflow-preview compatibility-only export parity", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("re-exports the canonical factory preview API surface under compatibility-only names", () => {
    expect(previewWorkflow).toBeTypeOf("function");
    expect(WorkflowPreviewAPIError).toBe(factoryPreview.FactoryPreviewAPIError);
    expect(workflowPreviewAPIErrorMessages.network).toBeTruthy();
  });

  it("delegates compatibility-only previewWorkflow to previewFactory", async () => {
    const previewFactorySpy = vi
      .spyOn(factoryPreview, "previewFactory")
      .mockResolvedValue({
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
      });

    await previewWorkflow({
      sourceKind: "WORKFLOW_NAME",
      projectRoot: "/tmp/project",
      sourceValue: "review",
    });

    expect(previewFactorySpy).toHaveBeenCalledWith(
      {
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "review",
      },
      {},
    );
  });
});

import type { components, paths } from "../generated/openapi";
import type {
  FactoryPreviewDiagnostic,
  FactoryPreviewRequest,
  FactoryPreviewResult,
} from "./api";

type GeneratedFactoryPreviewRequest =
  components["schemas"]["FactoryPreviewRequest"];
type GeneratedFactoryPreviewResult =
  components["schemas"]["FactoryPreviewResult"];
type GeneratedWorkflowPreviewRequest =
  components["schemas"]["WorkflowPreviewRequest"];
type GeneratedWorkflowPreviewResult =
  components["schemas"]["WorkflowPreviewResult"];

type CanonicalPreviewPath = paths["/factories/preview"]["post"];
type ObsoletePreviewPath = paths["/workflow-previews"]["post"];

describe("factory-preview generated types", () => {
  it("uses FactoryPreviewRequest and FactoryPreviewResult as the canonical preview models", () => {
    const request: FactoryPreviewRequest = {
      sourceKind: "WORKFLOW_NAME",
      projectRoot: "/tmp/project",
      sourceValue: "review",
    };
    const result: FactoryPreviewResult = {
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
    };

    const generatedRequest: GeneratedFactoryPreviewRequest = request;
    const generatedResult: GeneratedFactoryPreviewResult = result;

    expect(generatedRequest.sourceKind).toBe("WORKFLOW_NAME");
    expect(generatedResult.valid).toBe(true);
  });

  it("maps diagnostics through the shared WorkflowDiagnostic schema", () => {
    const diagnostic: FactoryPreviewDiagnostic = {
      code: "workflow.source.syntaxError",
      message: "unexpected token",
      path: "orchestrator.javascript",
      line: 3,
      column: 5,
    };

    const generatedDiagnostic: components["schemas"]["WorkflowDiagnostic"] =
      diagnostic;

    expect(generatedDiagnostic.path).toBe("orchestrator.javascript");
  });

  it("keeps WorkflowPreview schemas as obsolete aliases of the Factory preview models", () => {
    const request: GeneratedWorkflowPreviewRequest = {
      sourceKind: "INLINE_WORKFLOW",
      projectRoot: "/tmp/project",
      inlineSource: "phase('setup');",
    };
    const result: GeneratedWorkflowPreviewResult = {
      valid: false,
      sourceResolution: {
        found: true,
        requestKind: "INLINE_WORKFLOW",
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

    const factoryRequest: GeneratedFactoryPreviewRequest = request;
    const factoryResult: GeneratedFactoryPreviewResult = result;

    expect(factoryRequest.inlineSource).toBe("phase('setup');");
    expect(factoryResult.valid).toBe(false);
  });

  it("exposes canonical previewFactory and obsolete previewWorkflow operations", () => {
    const canonicalRequest: CanonicalPreviewPath["requestBody"] = {
      content: {
        "application/json": {
          sourceKind: "WORKFLOW_NAME",
          projectRoot: "/tmp/project",
          sourceValue: "review",
        },
      },
    };
    const obsoleteRequest: ObsoletePreviewPath["requestBody"] = {
      content: {
        "application/json": {
          sourceKind: "WORKFLOW_NAME",
          projectRoot: "/tmp/project",
          sourceValue: "review",
        },
      },
    };

    expect(
      canonicalRequest.content["application/json"].sourceKind,
    ).toBe("WORKFLOW_NAME");
    expect(
      obsoleteRequest.content["application/json"].sourceKind,
    ).toBe("WORKFLOW_NAME");
  });
});

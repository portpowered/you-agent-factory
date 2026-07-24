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
type CanonicalPreviewPath = paths["/factories/preview"]["post"];

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

  it("exposes the canonical previewFactory operation", () => {
    const canonicalRequest: CanonicalPreviewPath["requestBody"] = {
      content: {
        "application/json": {
          sourceKind: "WORKFLOW_NAME",
          projectRoot: "/tmp/project",
          sourceValue: "review",
        },
      },
    };
    expect(canonicalRequest.content["application/json"].sourceKind).toBe(
      "WORKFLOW_NAME",
    );
  });
});

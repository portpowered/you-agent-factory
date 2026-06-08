import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import {
  WorkflowPreviewAPIError,
  previewWorkflow,
  workflowPreviewAPIErrorMessages,
} from "../../../api/workflow-preview";
import { WorkflowPreviewPanel } from "./workflow-preview-panel";

vi.mock("../../../api/workflow-preview", async () => {
  const actual = await vi.importActual("../../../api/workflow-preview");
  return {
    ...actual,
    previewWorkflow: vi.fn(),
  };
});

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe("WorkflowPreviewPanel", () => {
  beforeEach(() => {
    vi.mocked(previewWorkflow).mockReset();
  });

  it("shows an empty state before a workflow request is provided", () => {
    render(
      <WorkflowPreviewPanel
        projectRoot="/tmp/project"
        sourceKind="WORKFLOW_NAME"
      />,
      { wrapper: createWrapper() },
    );

    expect(screen.getByTestId("workflow-preview-empty")).toBeTruthy();
    expect(previewWorkflow).not.toHaveBeenCalled();
  });

  it("shows loading and then success preview data", async () => {
    vi.mocked(previewWorkflow).mockResolvedValue({
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
    });

    render(
      <WorkflowPreviewPanel
        projectRoot="/tmp/project"
        sourceKind="WORKFLOW_NAME"
        sourceValue="review"
      />,
      { wrapper: createWrapper() },
    );

    expect(screen.getByTestId("workflow-preview-loading")).toBeTruthy();

    await waitFor(() => {
      expect(screen.getByTestId("workflow-preview-success")).toBeTruthy();
    });

    expect(screen.getByText("Workflow preview passed.")).toBeTruthy();
    expect(screen.getByText(/Source hash: sha256:abc/)).toBeTruthy();
    expect(screen.getByText(/Policy hash: sha256:policy/)).toBeTruthy();
  });

  it("shows validation and denied capability diagnostics on failure", async () => {
    vi.mocked(previewWorkflow).mockResolvedValue({
      valid: false,
      sourceResolution: {
        found: true,
        requestKind: "WORKFLOW_NAME",
        diagnostics: [],
      },
      sourceValidationIssues: [
        {
          code: "workflow.source.forbiddenHostAccess",
          message: "direct host access is forbidden",
          path: "orchestrator.javascript",
        },
      ],
      policyPreview: {
        effectivePolicy: { mode: "READ_ONLY" },
        policyHash: "sha256:policy",
        maxChildCount: 16,
        maxConcurrency: 4,
        deniedCapabilities: [
          {
            code: "workflow.policy.deniedCapability",
            message: "network access denied",
          },
        ],
        validationIssues: [],
      },
      resultConstraints: {
        requiresStructuredCloneableJson: true,
        artifactUriScheme: "you-artifact",
        maxEmbeddedBytes: 65536,
        rejectedValueKinds: ["function"],
      },
    });

    render(
      <WorkflowPreviewPanel
        projectRoot="/tmp/project"
        sourceKind="WORKFLOW_NAME"
        sourceValue="unsafe"
      />,
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(screen.getByTestId("workflow-preview-error")).toBeTruthy();
    });

    expect(screen.getByText("Workflow preview failed.")).toBeTruthy();
    expect(
      screen.getByText(/workflow.source.forbiddenHostAccess/),
    ).toBeTruthy();
    expect(screen.getByText("Denied capabilities")).toBeTruthy();
    expect(screen.getByText(/network access denied/)).toBeTruthy();
  });

  it("shows API failures from the preview query", async () => {
    vi.mocked(previewWorkflow).mockRejectedValue(
      new WorkflowPreviewAPIError(workflowPreviewAPIErrorMessages.network, {
        code: "NETWORK_ERROR",
      }),
    );

    render(
      <WorkflowPreviewPanel
        projectRoot="/tmp/project"
        sourceKind="WORKFLOW_NAME"
        sourceValue="review"
      />,
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(screen.getByTestId("workflow-preview-error")).toBeTruthy();
    });

    expect(
      screen.getByText(workflowPreviewAPIErrorMessages.network),
    ).toBeTruthy();
  });

  it("renders source resolution and policy diagnostics with locations", async () => {
    vi.mocked(previewWorkflow).mockResolvedValue({
      valid: false,
      sourceResolution: {
        found: false,
        requestKind: "WORKFLOW_NAME",
        diagnostics: [
          {
            code: "workflow.source.notFound",
            message: "workflow was not found",
          },
        ],
      },
      sourceValidationIssues: [
        {
          code: "workflow.source.syntaxError",
          message: "syntax error",
          path: "orchestrator.javascript",
          line: 3,
          column: 5,
        },
      ],
      policyPreview: {
        effectivePolicy: { mode: "READ_ONLY" },
        policyHash: "sha256:policy",
        maxChildCount: 16,
        maxConcurrency: 4,
        deniedCapabilities: [],
        validationIssues: [
          {
            code: "workflow.policy.invalidConcurrency",
            message: "concurrency must be positive",
            path: "policy.concurrency",
          },
        ],
      },
      resultConstraints: {
        requiresStructuredCloneableJson: true,
        artifactUriScheme: "you-artifact",
        maxEmbeddedBytes: 65536,
        rejectedValueKinds: ["function"],
      },
    });

    render(
      <WorkflowPreviewPanel
        projectRoot="/tmp/project"
        sourceKind="INLINE_WORKFLOW"
        inlineSource="phase('setup');"
      />,
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(screen.getByText("Source resolution")).toBeTruthy();
    });

    expect(screen.getByText(/workflow.source.notFound/)).toBeTruthy();
    expect(screen.getByText(/line 3, column 5/)).toBeTruthy();
  });

  it("formats diagnostics with line-only locations", async () => {
    vi.mocked(previewWorkflow).mockResolvedValue({
      valid: false,
      sourceResolution: {
        found: true,
        requestKind: "WORKFLOW_NAME",
      },
      sourceValidationIssues: [
        {
          code: "workflow.source.syntaxError",
          message: "unexpected token",
          line: 9,
        },
      ],
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

    render(
      <WorkflowPreviewPanel
        projectRoot="/tmp/project"
        sourceKind="WORKFLOW_NAME"
        sourceValue="broken"
      />,
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(screen.getByText(/\(line 9\)/)).toBeTruthy();
    });
  });
});

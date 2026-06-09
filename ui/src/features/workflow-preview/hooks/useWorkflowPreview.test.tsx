import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import {
  type WorkflowPreviewResult,
  previewWorkflow,
} from "../../../api/workflow-preview";
import {
  buildWorkflowPreviewQueryKey,
  useWorkflowPreview,
  workflowPreviewQueryOptions,
} from "./useWorkflowPreview";

vi.mock("../../../api/workflow-preview", async () => {
  const actual = await vi.importActual("../../../api/workflow-preview");
  return {
    ...actual,
    previewWorkflow: vi.fn(),
  };
});

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

describe("buildWorkflowPreviewQueryKey", () => {
  it("builds a stable query key for one preview request", () => {
    expect(
      buildWorkflowPreviewQueryKey({
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "review",
      }),
    ).toEqual([
      "workflow-preview",
      "WORKFLOW_NAME",
      "/tmp/project",
      "review",
      "",
      "",
    ]);
  });

  it("includes inline source and artifact root segments in the query key", () => {
    expect(
      buildWorkflowPreviewQueryKey({
        sourceKind: "INLINE_WORKFLOW",
        projectRoot: "/tmp/project",
        inlineSource: "phase('setup');",
        artifactRoot: "/tmp/artifacts",
      }),
    ).toEqual([
      "workflow-preview",
      "INLINE_WORKFLOW",
      "/tmp/project",
      "",
      "phase('setup');",
      "/tmp/artifacts",
    ]);
  });

  it("fills omitted optional request fields with empty query-key segments", () => {
    expect(
      buildWorkflowPreviewQueryKey({
        sourceKind: "WORKFLOW_FILE",
      }),
    ).toEqual(["workflow-preview", "WORKFLOW_FILE", "", "", "", ""]);
  });
});

describe("workflowPreviewQueryOptions", () => {
  it("throws when fetchQuery runs without a request", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
    });

    await expect(
      queryClient.fetchQuery(workflowPreviewQueryOptions(null)),
    ).rejects.toThrow("workflow preview request is required");
  });
});

describe("useWorkflowPreview", () => {
  beforeEach(() => {
    vi.mocked(previewWorkflow).mockReset();
    vi.mocked(previewWorkflow).mockResolvedValue(previewResult);
  });

  it("does not fetch when the request is null", () => {
    const { result } = renderHook(() => useWorkflowPreview(null), {
      wrapper: createWrapper(),
    });

    expect(previewWorkflow).not.toHaveBeenCalled();
    expect(result.current.status).toBe("pending");
  });

  it("does not fetch when the query is disabled", () => {
    const request = {
      sourceKind: "WORKFLOW_NAME" as const,
      projectRoot: "/tmp/project",
      sourceValue: "review",
    };

    renderHook(() => useWorkflowPreview(request, false), {
      wrapper: createWrapper(),
    });

    expect(previewWorkflow).not.toHaveBeenCalled();
  });

  it("fetches preview data for one workflow request", async () => {
    const request = {
      sourceKind: "WORKFLOW_NAME" as const,
      projectRoot: "/tmp/project",
      sourceValue: "review",
    };

    const { result } = renderHook(() => useWorkflowPreview(request), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.status).toBe("success");
    });

    expect(previewWorkflow).toHaveBeenCalledWith(request);
    expect(result.current.data?.valid).toBe(true);
  });
});

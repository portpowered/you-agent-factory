import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import {
  type FactoryPreviewResult,
  previewFactory,
} from "../../../api/factory-preview";
import {
  buildFactoryPreviewQueryKey,
  factoryPreviewQueryOptions,
  useFactoryPreview,
} from "./useWorkflowPreview";

vi.mock("../../../api/factory-preview", async () => {
  const actual = await vi.importActual("../../../api/factory-preview");
  return {
    ...actual,
    previewFactory: vi.fn(),
  };
});

const previewResult: FactoryPreviewResult = {
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

describe("buildFactoryPreviewQueryKey", () => {
  it("builds a stable query key for one preview request", () => {
    expect(
      buildFactoryPreviewQueryKey({
        sourceKind: "WORKFLOW_NAME",
        projectRoot: "/tmp/project",
        sourceValue: "review",
      }),
    ).toEqual([
      "factory-preview",
      "WORKFLOW_NAME",
      "/tmp/project",
      "review",
      "",
      "",
    ]);
  });

  it("includes inline source and artifact root segments in the query key", () => {
    expect(
      buildFactoryPreviewQueryKey({
        sourceKind: "INLINE_WORKFLOW",
        projectRoot: "/tmp/project",
        inlineSource: "phase('setup');",
        artifactRoot: "/tmp/artifacts",
      }),
    ).toEqual([
      "factory-preview",
      "INLINE_WORKFLOW",
      "/tmp/project",
      "",
      "phase('setup');",
      "/tmp/artifacts",
    ]);
  });

  it("fills omitted optional request fields with empty query-key segments", () => {
    expect(
      buildFactoryPreviewQueryKey({
        sourceKind: "WORKFLOW_FILE",
      }),
    ).toEqual(["factory-preview", "WORKFLOW_FILE", "", "", "", ""]);
  });
});

describe("factoryPreviewQueryOptions", () => {
  it("throws when fetchQuery runs without a request", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
    });

    await expect(
      queryClient.fetchQuery(factoryPreviewQueryOptions(null)),
    ).rejects.toThrow("factory preview request is required");
  });
});

describe("useFactoryPreview", () => {
  beforeEach(() => {
    vi.mocked(previewFactory).mockReset();
    vi.mocked(previewFactory).mockResolvedValue(previewResult);
  });

  it("does not fetch when the request is null", () => {
    const { result } = renderHook(() => useFactoryPreview(null), {
      wrapper: createWrapper(),
    });

    expect(previewFactory).not.toHaveBeenCalled();
    expect(result.current.status).toBe("pending");
  });

  it("does not fetch when the query is disabled", () => {
    const request = {
      sourceKind: "WORKFLOW_NAME" as const,
      projectRoot: "/tmp/project",
      sourceValue: "review",
    };

    renderHook(() => useFactoryPreview(request, false), {
      wrapper: createWrapper(),
    });

    expect(previewFactory).not.toHaveBeenCalled();
  });

  it("fetches preview data for one workflow request", async () => {
    const request = {
      sourceKind: "WORKFLOW_NAME" as const,
      projectRoot: "/tmp/project",
      sourceValue: "review",
    };

    const { result } = renderHook(() => useFactoryPreview(request), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.status).toBe("success");
    });

    expect(previewFactory).toHaveBeenCalledWith(request);
    expect(result.current.data?.valid).toBe(true);
  });
});

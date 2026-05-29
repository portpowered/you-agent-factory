import { renderHook } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/public";
import { useWorkerDetailState } from "./use-worker-detail-state";

vi.mock("../../../current-factory-definition/public", async () => {
  const actual = await vi.importActual(
    "../../../current-factory-definition/public",
  );

  return {
    ...actual,
    useCurrentFactoryDocument: vi.fn(),
  };
});

function mockFactoryDocumentQuery(
  overrides: Partial<ReturnType<typeof useCurrentFactoryDocument>> = {},
) {
  vi.mocked(useCurrentFactoryDocument).mockReturnValue({
    data: undefined,
    error: null,
    failureCount: 0,
    failureReason: null,
    fetchStatus: "idle",
    isError: false,
    isFetched: false,
    isFetchedAfterMount: false,
    isFetching: false,
    isInitialLoading: false,
    isLoading: false,
    isLoadingError: false,
    isPaused: false,
    isPending: true,
    isPlaceholderData: false,
    isRefetchError: false,
    isRefetching: false,
    isStale: true,
    isSuccess: false,
    promise: Promise.resolve(undefined),
    refetch: vi.fn(),
    status: "pending",
    ...overrides,
  } as never);
}

function buildFactoryDocument(
  overrides?: Partial<CurrentFactoryDocument>,
): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: {
      logical: "7",
      physical: "2026-05-23T16:22:24Z",
    },
    workers: [
      {
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [{ id: "review", name: "Review", worker: "reviewer" }],
    workTypes: [],
    ...overrides,
  };
}

describe("useWorkerDetailState", () => {
  beforeEach(() => {
    mockFactoryDocumentQuery();
  });

  it("returns loading while the current factory document is pending", () => {
    const { result } = renderHook(() => useWorkerDetailState("reviewer"));

    expect(result.current).toEqual({ status: "loading" });
  });

  it("returns error when the current factory document query fails", () => {
    mockFactoryDocumentQuery({
      error: { message: "Factory unavailable" },
      isError: true,
      isPending: false,
      status: "error",
    } as never);

    const { result } = renderHook(() => useWorkerDetailState("reviewer"));

    expect(result.current).toEqual({
      errorMessage: "Factory unavailable",
      status: "error",
    });
  });

  it("returns empty when the factory document has no data", () => {
    mockFactoryDocumentQuery({
      data: undefined,
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    const { result } = renderHook(() => useWorkerDetailState("reviewer"));

    expect(result.current).toEqual({ status: "empty" });
  });

  it("returns ready worker detail when the worker exists in the factory document", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    const { result } = renderHook(() => useWorkerDetailState("reviewer"));

    expect(result.current).toEqual({
      status: "ready",
      worker: {
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
      workstationNames: ["Review"],
    });
  });
});

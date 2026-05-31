import { render, screen } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { ResourceDetailCard } from "./resource-detail-card";

vi.mock(
  "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

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
    resources: [
      {
        backend: "llama.cpp",
        capacity: 1,
        loadPolicy: "ON_DEMAND",
        model: "OMNIVOICE_Q4_K_M",
        name: "voice-model",
        provider: "anthropic",
        type: "MODEL",
      },
      {
        capacity: 2,
        name: "agent-slot",
        type: "INVOCATION_SLOT",
      },
    ],
    workers: [
      {
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "reviewer",
        resources: [{ capacity: 1, name: "agent-slot" }],
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        id: "review",
        name: "Review",
        resources: [{ capacity: 1, name: "agent-slot" }],
        worker: "reviewer",
      },
    ],
    workTypes: [],
    ...overrides,
  };
}

describe("ResourceDetailCard", () => {
  beforeEach(() => {
    mockFactoryDocumentQuery();
  });

  it("shows loading state while the current factory document is pending", () => {
    render(<ResourceDetailCard resourceName="agent-slot" />);

    expect(
      screen.getByText(
        "Loading the current factory definition for this resource.",
      ),
    ).toBeTruthy();
  });

  it("shows error state when the current factory document fails to load", () => {
    mockFactoryDocumentQuery({
      error: { message: "Factory unavailable" },
      isError: true,
      isPending: false,
      status: "error",
    } as never);

    render(<ResourceDetailCard resourceName="agent-slot" />);

    expect(screen.getByRole("alert").textContent).toContain(
      "Resource definition unavailable.",
    );
    expect(screen.getByRole("alert").textContent).toContain(
      "Factory unavailable",
    );
  });

  it("shows empty state when the selected resource is missing from the factory document", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(<ResourceDetailCard resourceName="missing-resource" />);

    expect(
      screen.getByText(
        "This running factory definition does not include the selected resource.",
      ),
    ).toBeTruthy();
  });

  it("renders factory summary, runtime token count, and referencing lists", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <ResourceDetailCard resourceName="agent-slot" tokenCount={1} />,
    );

    expect(screen.getAllByText("agent-slot").length).toBeGreaterThan(0);
    expect(screen.getByRole("heading", { name: "Summary" })).toBeTruthy();
    expect(screen.getByText("Capacity")).toBeTruthy();
    expect(screen.getByText("2")).toBeTruthy();
    expect(screen.getByText("Invocation slot")).toBeTruthy();
    expect(screen.getByText("Available tokens")).toBeTruthy();
    expect(screen.getByText("1")).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Referencing workers" }),
    ).toBeTruthy();
    expect(screen.getByText("reviewer")).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Referencing workstations" }),
    ).toBeTruthy();
    expect(screen.getByText("Review")).toBeTruthy();
  });

  it("renders model-specific fields only for MODEL resources", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(<ResourceDetailCard resourceName="voice-model" />);

    expect(screen.getByText("OMNIVOICE_Q4_K_M")).toBeTruthy();
    expect(screen.getByText("llama.cpp")).toBeTruthy();
    expect(screen.getByText("ON_DEMAND")).toBeTruthy();
    expect(screen.queryByText("anthropic")).toBeNull();
  });
});

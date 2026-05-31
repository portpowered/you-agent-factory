// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: resource detail card regressions share one mocked factory-document seam.
import { fireEvent, render, screen } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { DashboardSelection } from "../../base/state/selection-types";
import { useEditableResourceConfigurationState } from "../hooks/use-editable-resource-configuration-state";
import type { EditableResourceConfigurationState } from "../lib/detail-card-types";
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

function renderResourceDetailCard(
  resourceName: string,
  options?: {
    selection?: DashboardSelection;
    tokenCount?: number | null;
  },
) {
  function Harness() {
    const editableConfigurationState = useEditableResourceConfigurationState(
      options?.selection ?? {
        kind: "resource",
        resourceName,
      },
      resourceName,
    );

    return (
      <ResourceDetailCard
        editableConfigurationState={editableConfigurationState}
        resourceName={resourceName}
        tokenCount={options?.tokenCount}
      />
    );
  }

  return render(<Harness />);
}

function renderReadOnlyResourceDetailCard(
  resourceName: string,
  options?: { tokenCount?: number | null },
) {
  return render(
    <ResourceDetailCard
      resourceName={resourceName}
      tokenCount={options?.tokenCount}
    />,
  );
}

describe("ResourceDetailCard", () => {
  beforeEach(() => {
    mockFactoryDocumentQuery();
  });

  it("shows loading state while the current factory document is pending", () => {
    renderResourceDetailCard("agent-slot");

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

    renderResourceDetailCard("agent-slot");

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

    renderResourceDetailCard("missing-resource");

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

    renderResourceDetailCard("agent-slot", { tokenCount: 1 });

    expect(
      screen.getByRole("heading", { name: "Resource configuration" }),
    ).toBeTruthy();
    expect(screen.getByLabelText("Name").getAttribute("value")).toBe(
      "agent-slot",
    );
    expect(screen.getByLabelText("Capacity").getAttribute("value")).toBe("2");
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

    renderResourceDetailCard("voice-model");

    expect(screen.getByLabelText("Model").getAttribute("value")).toBe(
      "OMNIVOICE_Q4_K_M",
    );
    expect(screen.getByLabelText("Backend").getAttribute("value")).toBe(
      "llama.cpp",
    );
    expect(screen.getByLabelText("Load policy").getAttribute("value")).toBe(
      "ON_DEMAND",
    );
    expect(screen.queryByText("anthropic")).toBeNull();
  });

  it("renders read-only context with empty referencing lists when nothing references the resource", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument({
        resources: [
          {
            capacity: 1,
            name: "orphan-slot",
            type: "INVOCATION_SLOT",
          },
        ],
        workers: [],
        workstations: [],
      }),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    renderReadOnlyResourceDetailCard("orphan-slot");

    expect(
      screen.getByText(
        "No workers in the running factory definition require this resource.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "No workstations in the running factory definition consume this resource.",
      ),
    ).toBeTruthy();
  });

  it("renders provider quota fields in read-only context", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument({
        resources: [
          {
            capacity: 10,
            name: "anthropic-quota",
            provider: "anthropic",
            type: "PROVIDER_QUOTA",
          },
        ],
      }),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    renderReadOnlyResourceDetailCard("anthropic-quota");

    expect(screen.getByText("Provider quota")).toBeTruthy();
    expect(screen.getByText("anthropic")).toBeTruthy();
  });

  it("collapses and expands editable resource configuration", async () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    renderResourceDetailCard("agent-slot");

    const collapseButton = screen.getByRole("button", {
      name: "Collapse resource configuration editor",
    });
    fireEvent.click(collapseButton);

    expect(screen.queryByLabelText("Name")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", {
        name: "Expand resource configuration editor",
      }),
    );

    expect(screen.getByLabelText("Name")).toBeTruthy();
  });

  it("renders provider quota editable fields for PROVIDER_QUOTA resources", () => {
    mockFactoryDocumentQuery({
      data: buildFactoryDocument({
        resources: [
          {
            capacity: 10,
            name: "anthropic-quota",
            provider: "anthropic",
            type: "PROVIDER_QUOTA",
          },
        ],
      }),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    renderResourceDetailCard("anthropic-quota");

    expect(screen.getByLabelText("Provider").getAttribute("value")).toBe(
      "anthropic",
    );
  });

  it("shows overwrite warning and server-changed hints for dirty resource drafts", () => {
    const editableConfigurationState: EditableResourceConfigurationState = {
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T16:22:24Z",
      },
      canSave: true,
      draft: {
        backend: "",
        capacityText: "2",
        loadPolicy: "",
        model: "",
        name: "agent-slot",
        provider: "",
        type: "INVOCATION_SLOT",
      },
      hasValidationErrors: false,
      initialValues: {
        backend: null,
        capacity: 2,
        loadPolicy: null,
        model: null,
        provider: null,
        resourceName: "agent-slot",
        type: "INVOCATION_SLOT",
        workerNames: ["reviewer"],
        workstationNames: ["Review"],
      },
      isDirty: true,
      markChangesSaved: vi.fn(),
      onBackendChange: vi.fn(),
      onCapacityChange: vi.fn(),
      onLoadPolicyChange: vi.fn(),
      onModelChange: vi.fn(),
      onNameChange: vi.fn(),
      onProviderChange: vi.fn(),
      onResetToLatest: vi.fn(),
      onTypeChange: vi.fn(),
      overwriteFieldNames: ["capacity"],
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {},
    };

    mockFactoryDocumentQuery({
      data: buildFactoryDocument(),
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    render(
      <ResourceDetailCard
        editableConfigurationState={editableConfigurationState}
        resourceName="agent-slot"
      />,
    );

    expect(screen.getByText(/overwrite newer server values for/i)).toBeTruthy();
    expect(
      screen.getByText(
        /The running factory changed this field while you were editing/i,
      ),
    ).toBeTruthy();
  });
});

// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: resource detail card regressions share one mocked factory-document seam.
// biome-ignore-all lint/style/noExcessiveLinesPerFile: resource detail card regressions share one mocked factory-document seam.
import { fireEvent, render, screen, within } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { DashboardSelection } from "../../base/state/selection-types";
import { useEditableResourceConfigurationState } from "../hooks/use-editable-resource-configuration-state";
import type {
  EditableResourceConfigurationState,
  EditableResourceSaveState,
} from "../lib/detail-card-types";
import { ResourceDetailCard } from "./resource-detail-card";
import { EditableResourceConfigurationHeaderActions } from "./resource-save-controls";

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

function buildResourceContext(
  resourceName: string,
  factoryDocument: CurrentFactoryDocument = buildFactoryDocument(),
) {
  return {
    resource:
      factoryDocument.resources?.find(
        (candidate) => candidate.name === resourceName,
      ) ?? undefined,
    workerNames:
      factoryDocument.workers
        ?.filter((worker) =>
          (worker.resources ?? []).some(
            (candidate) => candidate.name === resourceName,
          ),
        )
        .map((worker) => worker.name) ?? [],
    workstationNames:
      factoryDocument.workstations
        ?.filter((workstation) =>
          (workstation.resources ?? []).some(
            (candidate) => candidate.name === resourceName,
          ),
        )
        .map((workstation) => workstation.name) ?? [],
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
    const editableDefinition = useCurrentFactoryDocument().data ?? null;
    const fallbackDetailDefinition = buildFactoryDocument();
    const detailDefinition = editableDefinition ?? fallbackDetailDefinition;
    const resource =
      detailDefinition.resources?.find(
        (candidate) => candidate.name === resourceName,
      ) ?? null;
    const workerNames =
      detailDefinition.workers
        ?.filter((worker) =>
          (worker.resources ?? []).some(
            (candidate) => candidate.name === resourceName,
          ),
        )
        .map((worker) => worker.name) ?? [];
    const workstationNames =
      detailDefinition.workstations
        ?.filter((workstation) =>
          (workstation.resources ?? []).some(
            (candidate) => candidate.name === resourceName,
          ),
        )
        .map((workstation) => workstation.name) ?? [];
    const editableConfigurationState = useEditableResourceConfigurationState(
      options?.selection ?? {
        kind: "resource",
        resourceName,
      },
      resourceName,
      undefined,
      editableDefinition,
    );

    return (
      <ResourceDetailCard
        editableConfigurationState={editableConfigurationState}
        resource={resource}
        resourceName={resourceName}
        tokenCount={options?.tokenCount}
        workerNames={workerNames}
        workstationNames={workstationNames}
      />
    );
  }

  return render(<Harness />);
}

function resourceDetailHeaderActionSection() {
  const card = screen.getByRole("article", { name: "Current selection" });
  const undoButton = within(card).getByRole("button", {
    name: "Undo selection",
  });
  const actionSection = undoButton.closest(
    "[data-action-row-section='actions']",
  );
  if (!actionSection) {
    throw new Error("expected header action section");
  }

  return actionSection as HTMLElement;
}

function editableResourceConfigurationSection() {
  const heading = screen.getByRole("heading", {
    name: "Resource configuration",
  });
  const section = heading.closest("section");
  if (!section) {
    throw new Error("expected editable resource configuration section");
  }

  return section;
}

function buildResourceHeaderActions({
  canDiscard = false,
  canSave,
  onDiscard = vi.fn(),
  onSave = vi.fn(),
  saveState = { status: "idle" },
}: {
  canDiscard?: boolean;
  canSave: boolean;
  onDiscard?: () => void;
  onSave?: () => void;
  saveState?: EditableResourceSaveState;
}) {
  return (
    <EditableResourceConfigurationHeaderActions
      canDiscard={canDiscard}
      canSave={canSave}
      onDiscard={onDiscard}
      onSave={onSave}
      saveState={saveState}
    />
  );
}

function renderReadOnlyResourceDetailCard(
  resourceName: string,
  options?: { tokenCount?: number | null },
) {
  function Harness() {
    const factoryDocument =
      useCurrentFactoryDocument().data ?? buildFactoryDocument();
    const resourceContext = buildResourceContext(resourceName, factoryDocument);

    return (
      <ResourceDetailCard
        resourceName={resourceName}
        tokenCount={options?.tokenCount}
        {...resourceContext}
      />
    );
  }

  return render(<Harness />);
}

function expectPrimaryResourceTitle(resourceName: string) {
  const panel = screen.getByRole("article", { name: "Current selection" });
  const matchingText = within(panel).getAllByText(resourceName);

  expect(
    matchingText.some((element) =>
      element.classList.contains("type-display-large"),
    ),
  ).toBe(true);
}

describe("ResourceDetailCard", () => {
  beforeEach(() => {
    mockFactoryDocumentQuery();
  });

  it("shows loading state while the current factory document is pending", () => {
    renderResourceDetailCard("agent-slot");

    expectPrimaryResourceTitle("agent-slot");
    expect(
      screen
        .getByRole("button", {
          name: "Collapse resource configuration editor",
        })
        .getAttribute("aria-expanded"),
    ).toBe("true");
    expect(
      screen.getByText(
        "Loading editable resource configuration.",
      ),
    ).toBeTruthy();
  });

  it("shows error state when the current factory document fails to load", () => {
    render(
      <ResourceDetailCard
        editableConfigurationState={{
          errorMessage: "Factory unavailable",
          status: "error",
        }}
        resourceName="agent-slot"
        {...buildResourceContext("agent-slot")}
      />,
    );

    expect(screen.getByRole("alert").textContent).toContain(
      "Resource configuration unavailable.",
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

    expectPrimaryResourceTitle("agent-slot");
    expect(
      screen.getByRole("heading", { name: "Resource configuration" }),
    ).toBeTruthy();
    expect(screen.getByLabelText("Name").getAttribute("value")).toBe(
      "agent-slot",
    );
    expect(screen.getByLabelText("Capacity").getAttribute("value")).toBe("2");
    expect(screen.getAllByText("Invocation slot").length).toBeGreaterThan(0);
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
        {...buildResourceContext("agent-slot")}
      />,
    );

    expect(screen.getByText(/overwrite newer server values for/i)).toBeTruthy();
    expect(
      screen.getByText(
        /The running factory changed this field while you were editing/i,
      ),
    ).toBeTruthy();
  });

  it("keeps save and discard in the header only after dirty edits", () => {
    const editableConfigurationState: EditableResourceConfigurationState = {
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T16:22:24Z",
      },
      canSave: true,
      draft: {
        backend: "",
        capacityText: "3",
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
      overwriteFieldNames: [],
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

    const onSave = vi.fn();
    const onDiscard = vi.fn();

    render(
      <ResourceDetailCard
        editableConfigurationState={editableConfigurationState}
        headerAction={buildResourceHeaderActions({
          canDiscard: true,
          canSave: true,
          onDiscard,
          onSave,
        })}
        resourceName="agent-slot"
        {...buildResourceContext("agent-slot")}
      />,
    );

    const headerActions = resourceDetailHeaderActionSection();
    const saveButtons = within(headerActions).getAllByRole("button", {
      name: "Save resource",
    });
    const discardButtons = within(headerActions).getAllByRole("button", {
      name: "Discard local changes",
    });
    expect(saveButtons).toHaveLength(1);
    expect(discardButtons).toHaveLength(1);

    fireEvent.click(saveButtons[0]);
    expect(onSave).toHaveBeenCalledTimes(1);

    fireEvent.click(discardButtons[0]);
    expect(onDiscard).toHaveBeenCalledTimes(1);

    expect(
      within(editableResourceConfigurationSection()).queryByRole("button", {
        name: "Save resource",
      }),
    ).toBeNull();
    expect(
      within(editableResourceConfigurationSection()).queryByRole("button", {
        name: "Discard local changes",
      }),
    ).toBeNull();
  });

  it("omits global unsaved helper paragraphs for dirty ready-state resource drafts", () => {
    const editableConfigurationState: EditableResourceConfigurationState = {
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T16:22:24Z",
      },
      canSave: true,
      draft: {
        backend: "",
        capacityText: "3",
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
      overwriteFieldNames: [],
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
        headerAction={buildResourceHeaderActions({
          canDiscard: true,
          canSave: true,
        })}
        resourceName="agent-slot"
        {...buildResourceContext("agent-slot")}
      />,
    );

    expect(
      screen.queryByText("You have unsaved changes for this resource."),
    ).toBeNull();
    expect(
      screen.queryByText(
        "Changes stay local to this edit session until you save the running factory.",
      ),
    ).toBeNull();
    expect(screen.getByRole("button", { name: "Save resource" })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Discard local changes" }),
    ).toBeTruthy();
  });
});

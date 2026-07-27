import { describe, expect, it, mock } from "bun:test";
import { fireEvent, render, screen, within } from "@testing-library/react";

import type { FactoryResource } from "../../../../../api/events/types";
import type { EditableResourceConfigurationState } from "../../lib/detail-card-types";
import { ResourceDetailCard } from "../resource-detail-card";

const invocationSlot: FactoryResource = {
  capacity: 2,
  name: "agent-slot",
  type: "INVOCATION_SLOT",
};

const modelResource: FactoryResource = {
  backend: "llama.cpp",
  capacity: 1,
  loadPolicy: "ON_DEMAND",
  model: "OMNIVOICE_Q4_K_M",
  name: "voice-model",
  provider: "anthropic",
  type: "MODEL",
};

const providerQuota: FactoryResource = {
  capacity: 10,
  name: "anthropic-quota",
  provider: "anthropic",
  type: "PROVIDER_QUOTA",
};

describe("ResourceDetailCard owner contract", () => {
  it("renders editable loading, error, and missing-resource states", () => {
    const { rerender } = render(
      <ResourceDetailCard
        editableConfigurationState={{ status: "loading" }}
        resource={invocationSlot}
        resourceName={invocationSlot.name}
      />,
    );

    expect(
      screen.getByText("Loading editable resource configuration."),
    ).toBeTruthy();

    rerender(
      <ResourceDetailCard
        editableConfigurationState={{
          errorMessage: "Factory unavailable",
          status: "error",
        }}
        resource={invocationSlot}
        resourceName={invocationSlot.name}
      />,
    );
    expect(screen.getByRole("alert").textContent).toContain(
      "Resource configuration unavailable. Factory unavailable",
    );

    rerender(
      <ResourceDetailCard resource={null} resourceName="missing-resource" />,
    );
    expect(
      screen.getByText(
        "This running factory definition does not include the selected resource.",
      ),
    ).toBeTruthy();
  });

  it("renders invocation-slot fields, runtime context, and references", () => {
    render(
      <ResourceDetailCard
        editableConfigurationState={createReadyState(invocationSlot)}
        resource={invocationSlot}
        resourceName={invocationSlot.name}
        tokenCount={1}
        workerNames={["reviewer"]}
        workstationNames={["Review"]}
      />,
    );

    const card = screen.getByRole("article", { name: "Current selection" });
    expect(within(card).getByLabelText("Name").getAttribute("value")).toBe(
      "agent-slot",
    );
    expect(within(card).getByLabelText("Capacity").getAttribute("value")).toBe(
      "2",
    );
    expect(within(card).getAllByText("Invocation slot").length).toBeGreaterThan(
      0,
    );
    expect(within(card).getByText("Available tokens")).toBeTruthy();
    expect(within(card).getByText("reviewer")).toBeTruthy();
    expect(within(card).getByText("Review")).toBeTruthy();

    fireEvent.click(
      within(card).getByRole("button", {
        name: "Collapse resource configuration editor",
      }),
    );
    expect(within(card).queryByLabelText("Name")).toBeNull();
    fireEvent.click(
      within(card).getByRole("button", {
        name: "Expand resource configuration editor",
      }),
    );
    expect(within(card).getByLabelText("Name")).toBeTruthy();
  });
});

describe("ResourceDetailCard resource-type contracts", () => {
  it("renders only the fields owned by each specialized resource type", () => {
    const { rerender } = render(
      <ResourceDetailCard
        editableConfigurationState={createReadyState(modelResource)}
        resource={modelResource}
        resourceName={modelResource.name}
      />,
    );

    expect(screen.getByLabelText("Model").getAttribute("value")).toBe(
      "OMNIVOICE_Q4_K_M",
    );
    expect(screen.getByLabelText("Backend").getAttribute("value")).toBe(
      "llama.cpp",
    );
    expect(screen.getByLabelText("Load policy").getAttribute("value")).toBe(
      "ON_DEMAND",
    );
    expect(screen.queryByLabelText("Provider")).toBeNull();

    rerender(
      <ResourceDetailCard
        editableConfigurationState={createReadyState(providerQuota)}
        resource={providerQuota}
        resourceName={providerQuota.name}
      />,
    );
    expect(screen.getByLabelText("Provider").getAttribute("value")).toBe(
      "anthropic",
    );
    expect(screen.queryByLabelText("Model")).toBeNull();
  });

  it("renders read-only context without inventing references", () => {
    render(
      <ResourceDetailCard
        resource={providerQuota}
        resourceName={providerQuota.name}
      />,
    );

    expect(screen.getByText("Provider quota")).toBeTruthy();
    expect(screen.getByText("anthropic")).toBeTruthy();
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

  it("surfaces field-level server overwrite warnings for dirty drafts", () => {
    render(
      <ResourceDetailCard
        editableConfigurationState={createReadyState(invocationSlot, {
          isDirty: true,
          overwriteFieldNames: ["capacity"],
        })}
        resource={invocationSlot}
        resourceName={invocationSlot.name}
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

function createReadyState(
  resource: FactoryResource,
  overrides: Partial<
    Extract<EditableResourceConfigurationState, { status: "ready" }>
  > = {},
): Extract<EditableResourceConfigurationState, { status: "ready" }> {
  const factoryDefinition = {
    name: "Current Factory",
    resources: [resource],
    workers: [],
    workstations: [],
    workTypes: [],
  };

  return {
    baseVersion: {
      logical: "7",
      physical: "2026-05-23T16:22:24Z",
    },
    canSave: true,
    draft: {
      backend: resource.backend ?? "",
      capacityText: String(resource.capacity ?? ""),
      loadPolicy: resource.loadPolicy ?? "",
      model: resource.model ?? "",
      name: resource.name,
      provider: resource.provider ?? "",
      type: resource.type ?? null,
    },
    hasValidationErrors: false,
    initialValues: {
      backend: resource.backend ?? null,
      capacity: resource.capacity ?? null,
      loadPolicy: resource.loadPolicy ?? null,
      model: resource.model ?? null,
      provider: resource.provider ?? null,
      resourceName: resource.name,
      type: resource.type ?? null,
      workerNames: [],
      workstationNames: [],
    },
    isDirty: false,
    markChangesSaved: mock(() => {}),
    onBackendChange: mock(() => {}),
    onCapacityChange: mock(() => {}),
    onLoadPolicyChange: mock(() => {}),
    onModelChange: mock(() => {}),
    onNameChange: mock(() => {}),
    onProviderChange: mock(() => {}),
    onResetToLatest: mock(() => {}),
    onTypeChange: mock(() => {}),
    overwriteFieldNames: [],
    pendingFactoryDefinition: factoryDefinition,
    savedFactoryDefinition: factoryDefinition,
    status: "ready",
    validationErrors: {},
    ...overrides,
  };
}

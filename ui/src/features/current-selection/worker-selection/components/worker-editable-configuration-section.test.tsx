import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { CanonicalFactoryDefinition } from "../../../../api/factory-definition/api";
import type { EditableWorkerConfigurationState } from "../lib/detail-card-types";
import { getWorkerDetailMessages } from "../messages/worker-detail";
import { WorkerEditableConfigurationSection } from "./worker-editable-configuration-section";

const messages = getWorkerDetailMessages();

function buildReadyWorkerEditableConfigurationState(
  workstationNames: string[],
): Extract<EditableWorkerConfigurationState, { status: "ready" }> {
  return {
    canSave: true,
    draft: {
      argsText: "",
      body: "",
      command: "",
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CURSOR",
      name: "reviewer",
      provider: null,
      skipPermissions: false,
      stopToken: "",
      timeoutAmount: "",
      timeoutUnit: "m",
      type: "MODEL_WORKER",
    },
    hasValidationErrors: false,
    initialValues: {
      args: [],
      body: null,
      command: null,
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CURSOR",
      name: "reviewer",
      provider: null,
      skipPermissions: null,
      stopToken: null,
      timeout: null,
      type: "MODEL_WORKER",
      workerName: "reviewer",
      workstationNames,
    },
    isDirty: true,
    onArgsTextChange: vi.fn(),
    onBodyChange: vi.fn(),
    onCommandChange: vi.fn(),
    onExecutorProviderChange: vi.fn(),
    onModelChange: vi.fn(),
    onModelLocalityChange: vi.fn(),
    onModelProviderChange: vi.fn(),
    onNameChange: vi.fn(),
    onProviderChange: vi.fn(),
    onSkipPermissionsChange: vi.fn(),
    onStopTokenChange: vi.fn(),
    onTimeoutAmountChange: vi.fn(),
    onTimeoutUnitChange: vi.fn(),
    markChangesSaved: vi.fn(),
    onResetToLatest: vi.fn(),
    onTypeChange: vi.fn(),
    overwriteFieldNames: [],
    pendingFactoryDefinition: {} as CanonicalFactoryDefinition,
    status: "ready",
    validationErrors: {},
  };
}

describe("WorkerEditableConfigurationSection shared-impact warnings", () => {
  it("shows worker save-impact warning when multiple workstations reference the worker", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={buildReadyWorkerEditableConfigurationState(["Review", "Plan"])}
        workerName="reviewer"
      />,
    );

    expect(screen.getByRole("alert").textContent).toContain(
      "Saving reviewer updates workstations",
    );
    expect(screen.getByRole("alert").textContent).toMatch(/Review.*Plan/);
    expect(
      screen.queryByText(
        messages.editableConfigurationSharedImpactWarningDetail,
      ),
    ).toBeNull();
  });

  it("does not show worker save-impact warning for a single-workstation worker", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={buildReadyWorkerEditableConfigurationState(["Review"])}
        workerName="reviewer"
      />,
    );

    expect(
      screen.queryByText(/updates workstations/i),
    ).toBeNull();
  });
});

describe("WorkerEditableConfigurationSection timeout control", () => {
  it("shows the execution timeout picker for all worker types", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={buildReadyWorkerEditableConfigurationState(["Review"])}
        workerName="reviewer"
      />,
    );

    expect(
      screen.getByRole("spinbutton", { name: messages.timeoutFieldLabel }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("combobox", { name: messages.timeoutFieldLabel }),
    ).toBeInTheDocument();
  });

  it("shows the timeout picker for script workers", () => {
    const scriptWorkerState: Extract<
      EditableWorkerConfigurationState,
      { status: "ready" }
    > = {
      ...buildReadyWorkerEditableConfigurationState(["Review"]),
      draft: {
        ...buildReadyWorkerEditableConfigurationState(["Review"]).draft,
        model: "",
        modelProvider: null,
        type: "SCRIPT_WORKER",
        command: "node",
      },
    };

    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={scriptWorkerState}
        workerName="reviewer"
      />,
    );

    expect(
      screen.getByRole("spinbutton", { name: messages.timeoutFieldLabel }),
    ).toBeInTheDocument();
  });
});

describe("WorkerEditableConfigurationSection skipPermissions control", () => {
  it("shows the permission bypass toggle for model workers", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={buildReadyWorkerEditableConfigurationState(["Review"])}
        workerName="reviewer"
      />,
    );

    expect(
      screen.getByRole("checkbox", {
        name: messages.skipPermissionsFieldLabel,
      }),
    ).toBeInTheDocument();
  });

  it("does not show the permission bypass toggle for script workers", () => {
    const scriptWorkerState: Extract<
      EditableWorkerConfigurationState,
      { status: "ready" }
    > = {
      ...buildReadyWorkerEditableConfigurationState(["Review"]),
      draft: {
        ...buildReadyWorkerEditableConfigurationState(["Review"]).draft,
        model: "",
        modelProvider: null,
        type: "SCRIPT_WORKER",
        command: "node",
      },
    };

    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={scriptWorkerState}
        workerName="reviewer"
      />,
    );

    expect(
      screen.queryByRole("checkbox", {
        name: messages.skipPermissionsFieldLabel,
      }),
    ).toBeNull();
  });
});

describe("WorkerEditableConfigurationSection stopToken control", () => {
  it("shows the stop token input for all worker types", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={buildReadyWorkerEditableConfigurationState(["Review"])}
        workerName="reviewer"
      />,
    );

    expect(
      screen.getByRole("textbox", { name: messages.stopTokenFieldLabel }),
    ).toBeInTheDocument();
    expect(screen.getByText(messages.stopTokenFieldHelp)).toBeInTheDocument();
  });

  it("shows the stop token input for script workers", () => {
    const scriptWorkerState: Extract<
      EditableWorkerConfigurationState,
      { status: "ready" }
    > = {
      ...buildReadyWorkerEditableConfigurationState(["Review"]),
      draft: {
        ...buildReadyWorkerEditableConfigurationState(["Review"]).draft,
        model: "",
        modelProvider: null,
        type: "SCRIPT_WORKER",
        command: "node",
      },
    };

    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={scriptWorkerState}
        workerName="reviewer"
      />,
    );

    expect(
      screen.getByRole("textbox", { name: messages.stopTokenFieldLabel }),
    ).toBeInTheDocument();
  });
});

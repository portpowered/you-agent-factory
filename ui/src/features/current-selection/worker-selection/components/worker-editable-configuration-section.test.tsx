import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";

import type { EditableWorkerConfigurationState } from "../lib/detail-card-types";
import { WorkerEditableConfigurationSection } from "./worker-editable-configuration-section";
import {
  buildReadyWorkerEditableConfigurationState,
  workerEditableConfigurationSectionMessages as messages,
} from "./worker-editable-configuration-section.test-helpers";

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

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

    expect(screen.queryByText(/updates workstations/i)).toBeNull();
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

function buildHostedLinearWorkerEditableConfigurationState(): Extract<
  EditableWorkerConfigurationState,
  { status: "ready" }
> {
  return {
    ...buildReadyWorkerEditableConfigurationState(["Sync"]),
    canSave: true,
    draft: {
      ...buildReadyWorkerEditableConfigurationState(["Sync"]).draft,
      model: "",
      modelProvider: null,
      name: "linear-poller",
      provider: "LINEAR",
      type: "HOSTED_WORKER",
      authSecretRef: "secrets/linear-api-key",
      linearClaimAssigneeField: "assignee.email",
      linearMappingState: "queued",
      linearMappingWorkType: "story",
      linearPollInterval: "30s",
      linearStateIdsText: "state-a",
      linearTeamIdsText: "team-a",
    },
    hasValidationErrors: false,
    initialValues: {
      ...buildReadyWorkerEditableConfigurationState(["Sync"]).initialValues,
      model: null,
      modelProvider: null,
      provider: "LINEAR",
      type: "HOSTED_WORKER",
      workerName: "linear-poller",
      workstationNames: ["Sync"],
      authSecretRef: "secrets/linear-api-key",
      linearClaimAssigneeField: "assignee.email",
      linearClaimPresent: true,
      linearMappingState: "queued",
      linearMappingWorkType: "story",
      linearPollInterval: "30s",
      linearStateIds: ["state-a"],
      linearTeamIds: ["team-a"],
    },
    isDirty: false,
    validationErrors: {},
  };
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: hosted Linear field rendering regressions stay grouped in one section harness.
describe("WorkerEditableConfigurationSection hosted Linear poller fields", () => {
  it("renders hosted Linear poller inputs when provider is LINEAR", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={buildHostedLinearWorkerEditableConfigurationState()}
        workerName="linear-poller"
      />,
    );

    expect(
      screen.getByRole("textbox", { name: messages.authSecretRefFieldLabel }),
    ).toHaveValue("secrets/linear-api-key");
    expect(
      screen.getByRole("textbox", {
        name: messages.linearPollIntervalFieldLabel,
      }),
    ).toHaveValue("30s");
    expect(
      screen.getByRole("textbox", { name: messages.linearTeamIdsFieldLabel }),
    ).toHaveValue("team-a");
    expect(
      screen.getByRole("textbox", { name: messages.linearStateIdsFieldLabel }),
    ).toHaveValue("state-a");
    expect(
      screen.getByRole("textbox", {
        name: messages.linearMappingWorkTypeFieldLabel,
      }),
    ).toHaveValue("story");
    expect(
      screen.getByRole("textbox", {
        name: messages.linearMappingStateFieldLabel,
      }),
    ).toHaveValue("queued");
    expect(
      screen.getByRole("textbox", {
        name: messages.linearClaimAssigneeFieldLabel,
      }),
    ).toHaveValue("assignee.email");
    expect(
      screen.getByText(messages.authSecretRefFieldHelp),
    ).toBeInTheDocument();
  });

  it("calls hosted Linear draft handlers when fields change", () => {
    const state = buildHostedLinearWorkerEditableConfigurationState();

    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={state}
        workerName="linear-poller"
      />,
    );

    fireEvent.change(
      screen.getByRole("textbox", { name: messages.authSecretRefFieldLabel }),
      { target: { value: "secrets/other-key" } },
    );
    fireEvent.change(
      screen.getByRole("textbox", {
        name: messages.linearMappingWorkTypeFieldLabel,
      }),
      { target: { value: "task" } },
    );

    expect(state.onAuthSecretRefChange).toHaveBeenCalledWith(
      "secrets/other-key",
    );
    expect(state.onLinearMappingWorkTypeChange).toHaveBeenCalledWith("task");
  });

  it("shows hosted Linear validation errors on matching fields", () => {
    const state = {
      ...buildHostedLinearWorkerEditableConfigurationState(),
      canSave: false,
      hasValidationErrors: true,
      isDirty: true,
      validationErrors: {
        authSecretRef: messages.editableConfigurationAuthSecretRefRequired,
        linearMappingWorkType:
          messages.editableConfigurationLinearMappingWorkTypeRequired,
      },
    };

    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={state}
        workerName="linear-poller"
      />,
    );

    expect(
      screen.getByText(messages.editableConfigurationAuthSecretRefRequired),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        messages.editableConfigurationLinearMappingWorkTypeRequired,
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        messages.editableConfigurationSaveDisabledValidationDetail,
      ),
    ).toBeInTheDocument();
  });

  it("does not render hosted Linear poller inputs when provider is unset", () => {
    const state = {
      ...buildHostedLinearWorkerEditableConfigurationState(),
      draft: {
        ...buildHostedLinearWorkerEditableConfigurationState().draft,
        provider: null,
      },
    };

    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={state}
        workerName="linear-poller"
      />,
    );

    expect(
      screen.queryByRole("textbox", { name: messages.authSecretRefFieldLabel }),
    ).toBeNull();
  });
});

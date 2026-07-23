import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../../../components/dashboard/test-browser-shims";
import { selectComboboxOption } from "../../../../../../testing/select-test-helpers";

import type { EditableWorkerConfigurationState } from "../../../lib/detail-card-types";
import {
  buildReadyWorkerEditableConfigurationState,
  workerEditableConfigurationSectionMessages as messages,
} from "../worker-editable-configuration-section.test-helpers";
import { WorkerEditableConfigurationHostedFields } from "./worker-editable-configuration-hosted-fields";

let restoreBrowserShims: (() => void) | undefined;

const HOSTED_LINEAR_WORKER_DRAFT = {
  authSecretRef: "secrets/linear-api-key",
  linearClaimAssigneeField: "assignee.email",
  linearMappingState: "queued",
  linearMappingWorkType: "story",
  linearPollInterval: "30s",
  linearStateIdsText: "state-a",
  linearTeamIdsText: "team-a",
  model: "",
  modelProvider: null,
  name: "linear-poller",
  provider: "LINEAR" as const,
  type: "HOSTED_WORKER" as const,
};

function renderHostedFields(
  stateOverrides: Partial<
    Extract<EditableWorkerConfigurationState, { status: "ready" }>
  > = {},
  validationErrors: Record<string, string> = {},
) {
  const state = {
    ...buildReadyWorkerEditableConfigurationState(["Sync"]),
    ...stateOverrides,
    draft: {
      ...buildReadyWorkerEditableConfigurationState(["Sync"]).draft,
      ...HOSTED_LINEAR_WORKER_DRAFT,
      ...stateOverrides.draft,
    },
    validationErrors: {
      ...buildReadyWorkerEditableConfigurationState(["Sync"]).validationErrors,
      ...validationErrors,
      ...stateOverrides.validationErrors,
    },
  };

  render(
    <WorkerEditableConfigurationHostedFields
      messages={messages}
      state={state}
      validationErrors={state.validationErrors}
    />,
  );

  return state;
}

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: hosted field rendering regressions stay grouped in one field-group harness.
describe("WorkerEditableConfigurationHostedFields", () => {
  it("renders hosted provider labels, values, and provider options", () => {
    renderHostedFields();

    expect(
      screen.getByRole("combobox", { name: messages.providerFieldLabel }),
    ).toHaveTextContent("LINEAR");
  });

  it("shows the not-configured placeholder when hosted provider is unset", () => {
    renderHostedFields({
      draft: {
        provider: null,
      },
    });

    expect(
      screen.getByRole("combobox", { name: messages.providerFieldLabel }),
    ).toHaveTextContent(messages.notConfiguredOptionLabel);
  });

  it("renders hosted Linear poller inputs when provider is LINEAR", () => {
    renderHostedFields();

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
    expect(
      screen.getByText(messages.linearPollIntervalFieldHelp),
    ).toBeInTheDocument();
    expect(
      screen.getByText(messages.linearTeamIdsFieldHelp),
    ).toBeInTheDocument();
    expect(
      screen.getByText(messages.linearStateIdsFieldHelp),
    ).toBeInTheDocument();
    expect(
      screen.getByText(messages.linearMappingWorkTypeFieldHelp),
    ).toBeInTheDocument();
    expect(
      screen.getByText(messages.linearMappingStateFieldHelp),
    ).toBeInTheDocument();
    expect(
      screen.getByText(messages.linearClaimAssigneeFieldFieldHelp),
    ).toBeInTheDocument();
  });

  it("does not render hosted Linear poller inputs when provider is unset", () => {
    renderHostedFields({
      draft: {
        provider: null,
      },
    });

    expect(
      screen.queryByRole("textbox", { name: messages.authSecretRefFieldLabel }),
    ).toBeNull();
    expect(
      screen.queryByRole("textbox", {
        name: messages.linearPollIntervalFieldLabel,
      }),
    ).toBeNull();
  });

  it("calls the hosted provider draft handler when the combobox value changes", async () => {
    const user = userEvent.setup();
    const state = renderHostedFields({
      draft: {
        provider: null,
      },
    });

    await selectComboboxOption(
      user,
      screen.getByRole("combobox", { name: messages.providerFieldLabel }),
      "LINEAR",
    );

    expect(state.onProviderChange).toHaveBeenCalledWith("LINEAR");
  });

  it("calls hosted Linear draft handlers when fields change", () => {
    const state = renderHostedFields();

    fireEvent.change(
      screen.getByRole("textbox", { name: messages.authSecretRefFieldLabel }),
      { target: { value: "secrets/other-key" } },
    );
    fireEvent.change(
      screen.getByRole("textbox", {
        name: messages.linearPollIntervalFieldLabel,
      }),
      { target: { value: "45s" } },
    );
    fireEvent.change(
      screen.getByRole("textbox", { name: messages.linearTeamIdsFieldLabel }),
      { target: { value: "team-b" } },
    );
    fireEvent.change(
      screen.getByRole("textbox", { name: messages.linearStateIdsFieldLabel }),
      { target: { value: "state-b" } },
    );
    fireEvent.change(
      screen.getByRole("textbox", {
        name: messages.linearMappingWorkTypeFieldLabel,
      }),
      { target: { value: "task" } },
    );
    fireEvent.change(
      screen.getByRole("textbox", {
        name: messages.linearMappingStateFieldLabel,
      }),
      { target: { value: "in-progress" } },
    );
    fireEvent.change(
      screen.getByRole("textbox", {
        name: messages.linearClaimAssigneeFieldLabel,
      }),
      { target: { value: "assignee.id" } },
    );

    expect(state.onAuthSecretRefChange).toHaveBeenCalledWith(
      "secrets/other-key",
    );
    expect(state.onLinearPollIntervalChange).toHaveBeenCalledWith("45s");
    expect(state.onLinearTeamIdsTextChange).toHaveBeenCalledWith("team-b");
    expect(state.onLinearStateIdsTextChange).toHaveBeenCalledWith("state-b");
    expect(state.onLinearMappingWorkTypeChange).toHaveBeenCalledWith("task");
    expect(state.onLinearMappingStateChange).toHaveBeenCalledWith(
      "in-progress",
    );
    expect(state.onLinearClaimAssigneeFieldChange).toHaveBeenCalledWith(
      "assignee.id",
    );
  });

  // biome-ignore lint/complexity/noExcessiveLinesPerFunction: the accessibility regression keeps the complete hosted-field error and aria relationship matrix together.
  it("shows validation errors with accessible ids and aria relationships", () => {
    renderHostedFields(
      {},
      {
        authSecretRef: messages.editableConfigurationAuthSecretRefRequired,
        linearClaimAssigneeField: "Invalid claim assignee field.",
        linearMappingState:
          messages.editableConfigurationLinearMappingStateRequired,
        linearMappingWorkType:
          messages.editableConfigurationLinearMappingWorkTypeRequired,
        linearPollInterval: "Invalid poll interval.",
        linearStateIds: "Invalid state ids.",
        linearTeamIds: "Invalid team ids.",
        provider: messages.editableConfigurationProviderRequired,
      },
    );

    const providerSelect = screen.getByRole("combobox", {
      name: messages.providerFieldLabel,
    });
    const authSecretRefInput = screen.getByRole("textbox", {
      name: messages.authSecretRefFieldLabel,
    });
    const linearPollIntervalInput = screen.getByRole("textbox", {
      name: messages.linearPollIntervalFieldLabel,
    });
    const linearTeamIdsInput = screen.getByRole("textbox", {
      name: messages.linearTeamIdsFieldLabel,
    });
    const linearStateIdsInput = screen.getByRole("textbox", {
      name: messages.linearStateIdsFieldLabel,
    });
    const linearMappingWorkTypeInput = screen.getByRole("textbox", {
      name: messages.linearMappingWorkTypeFieldLabel,
    });
    const linearMappingStateInput = screen.getByRole("textbox", {
      name: messages.linearMappingStateFieldLabel,
    });
    const linearClaimAssigneeFieldInput = screen.getByRole("textbox", {
      name: messages.linearClaimAssigneeFieldLabel,
    });

    expect(providerSelect).toHaveAttribute("aria-invalid", "true");
    expect(providerSelect).toHaveAttribute(
      "aria-describedby",
      "editable-worker-provider-error",
    );
    expect(authSecretRefInput).toHaveAttribute("aria-invalid", "true");
    expect(authSecretRefInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-auth-secret-ref-error",
    );
    expect(linearPollIntervalInput).toHaveAttribute("aria-invalid", "true");
    expect(linearPollIntervalInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-linear-poll-interval-error",
    );
    expect(linearTeamIdsInput).toHaveAttribute("aria-invalid", "true");
    expect(linearTeamIdsInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-linear-team-ids-error",
    );
    expect(linearStateIdsInput).toHaveAttribute("aria-invalid", "true");
    expect(linearStateIdsInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-linear-state-ids-error",
    );
    expect(linearMappingWorkTypeInput).toHaveAttribute("aria-invalid", "true");
    expect(linearMappingWorkTypeInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-linear-mapping-work-type-error",
    );
    expect(linearMappingStateInput).toHaveAttribute("aria-invalid", "true");
    expect(linearMappingStateInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-linear-mapping-state-error",
    );
    expect(linearClaimAssigneeFieldInput).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(linearClaimAssigneeFieldInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-linear-claim-assignee-field-error",
    );

    expect(
      screen.getByText(messages.editableConfigurationProviderRequired),
    ).toHaveAttribute("id", "editable-worker-provider-error");
    expect(
      screen.getByText(messages.editableConfigurationAuthSecretRefRequired),
    ).toHaveAttribute("id", "editable-worker-auth-secret-ref-error");
    expect(screen.getByText("Invalid poll interval.")).toHaveAttribute(
      "id",
      "editable-worker-linear-poll-interval-error",
    );
    expect(screen.getByText("Invalid team ids.")).toHaveAttribute(
      "id",
      "editable-worker-linear-team-ids-error",
    );
    expect(screen.getByText("Invalid state ids.")).toHaveAttribute(
      "id",
      "editable-worker-linear-state-ids-error",
    );
    expect(
      screen.getByText(
        messages.editableConfigurationLinearMappingWorkTypeRequired,
      ),
    ).toHaveAttribute("id", "editable-worker-linear-mapping-work-type-error");
    expect(
      screen.getByText(
        messages.editableConfigurationLinearMappingStateRequired,
      ),
    ).toHaveAttribute("id", "editable-worker-linear-mapping-state-error");
    expect(screen.getByText("Invalid claim assignee field.")).toHaveAttribute(
      "id",
      "editable-worker-linear-claim-assignee-field-error",
    );
  });

  it("uses help text ids for aria-describedby when hosted Linear fields are valid", () => {
    renderHostedFields();

    expect(
      screen.getByRole("textbox", { name: messages.authSecretRefFieldLabel }),
    ).toHaveAttribute(
      "aria-describedby",
      "editable-worker-auth-secret-ref-hint",
    );
    expect(
      screen.getByRole("textbox", {
        name: messages.linearPollIntervalFieldLabel,
      }),
    ).toHaveAttribute(
      "aria-describedby",
      "editable-worker-linear-poll-interval-hint",
    );
    expect(
      screen.getByRole("textbox", { name: messages.linearTeamIdsFieldLabel }),
    ).toHaveAttribute(
      "aria-describedby",
      "editable-worker-linear-team-ids-hint",
    );
    expect(
      screen.getByRole("textbox", { name: messages.linearStateIdsFieldLabel }),
    ).toHaveAttribute(
      "aria-describedby",
      "editable-worker-linear-state-ids-hint",
    );
  });

  it("shows server-change hints for overwritten hosted fields", () => {
    renderHostedFields({
      overwriteFieldNames: [
        "provider",
        "authSecretRef",
        "linearPollInterval",
        "linearTeamIds",
        "linearStateIds",
        "linearMappingWorkType",
        "linearMappingState",
        "linearClaimAssigneeField",
      ],
    });

    expect(
      screen.getAllByText(messages.editableConfigurationServerFieldChangedHint),
    ).toHaveLength(8);
  });

  it("keeps hosted Linear fields keyboard reachable", async () => {
    const user = userEvent.setup();
    renderHostedFields();

    const authSecretRefInput = screen.getByRole("textbox", {
      name: messages.authSecretRefFieldLabel,
    });
    const linearPollIntervalInput = screen.getByRole("textbox", {
      name: messages.linearPollIntervalFieldLabel,
    });
    const linearTeamIdsInput = screen.getByRole("textbox", {
      name: messages.linearTeamIdsFieldLabel,
    });
    const linearStateIdsInput = screen.getByRole("textbox", {
      name: messages.linearStateIdsFieldLabel,
    });

    expect(linearTeamIdsInput.tagName).toBe("TEXTAREA");
    expect(linearStateIdsInput.tagName).toBe("TEXTAREA");

    await user.click(authSecretRefInput);
    await user.tab();
    expect(linearPollIntervalInput).toHaveFocus();
    await user.tab();
    expect(linearTeamIdsInput).toHaveFocus();
    await user.tab();
    expect(linearStateIdsInput).toHaveFocus();
  });
});
